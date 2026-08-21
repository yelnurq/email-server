package main

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

type entry struct {
	value    string
	expires  time.Time
	hasTTL   bool
}

type store struct {
	mu   sync.Mutex
	data map[string]entry
}

func newStore() *store {
	return &store{data: map[string]entry{}}
}

func (s *store) cleanupLocked(key string) (entry, bool) {
	e, ok := s.data[key]
	if !ok {
		return entry{}, false
	}
	if e.hasTTL && time.Now().After(e.expires) {
		delete(s.data, key)
		return entry{}, false
	}
	return e, true
}

func (s *store) get(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.cleanupLocked(key)
	if !ok {
		return "", false
	}
	return e.value, true
}

func (s *store) set(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.data[key]
	e.value = value
	s.data[key] = e
}

func (s *store) incr(key string) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.cleanupLocked(key)
	n := int64(0)
	if ok {
		var err error
		n, err = strconv.ParseInt(e.value, 10, 64)
		if err != nil {
			return 0, err
		}
	}
	n++
	s.data[key] = entry{value: strconv.FormatInt(n, 10)}
	return n, nil
}

func (s *store) expire(key string, seconds int64) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.cleanupLocked(key)
	if !ok {
		return 0
	}
	e.hasTTL = true
	e.expires = time.Now().Add(time.Duration(seconds) * time.Second)
	s.data[key] = e
	return 1
}

func (s *store) del(key string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.cleanupLocked(key); !ok {
		return 0
	}
	delete(s.data, key)
	return 1
}

func (s *store) exists(key string) int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.cleanupLocked(key); !ok {
		return 0
	}
	return 1
}

func readLine(r *bufio.Reader) (string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), nil
}

func readCommand(r *bufio.Reader) ([]string, error) {
	b, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	if b != '*' {
		line, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		cmd := strings.TrimSpace(string(append([]byte{b}, []byte(line)...)))
		if cmd == "" {
			return nil, fmt.Errorf("empty command")
		}
		return strings.Fields(cmd), nil
	}
	countLine, err := readLine(r)
	if err != nil {
		return nil, err
	}
	n, err := strconv.Atoi(countLine)
	if err != nil {
		return nil, err
	}
	args := make([]string, 0, n)
	for i := 0; i < n; i++ {
		prefix, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if prefix != '$' {
			return nil, fmt.Errorf("expected bulk string")
		}
		ln, err := readLine(r)
		if err != nil {
			return nil, err
		}
		l, err := strconv.Atoi(ln)
		if err != nil {
			return nil, err
		}
		buf := make([]byte, l+2)
		if _, err := r.Read(buf); err != nil {
			return nil, err
		}
		args = append(args, string(buf[:l]))
	}
	return args, nil
}

func writeSimple(w *bufio.Writer, s string) error {
	_, err := fmt.Fprintf(w, "+%s\r\n", s)
	return err
}

func writeError(w *bufio.Writer, s string) error {
	_, err := fmt.Fprintf(w, "-ERR %s\r\n", s)
	return err
}

func writeInt(w *bufio.Writer, n int64) error {
	_, err := fmt.Fprintf(w, ":%d\r\n", n)
	return err
}

func writeBulk(w *bufio.Writer, s string) error {
	_, err := fmt.Fprintf(w, "$%d\r\n%s\r\n", len(s), s)
	return err
}

func writeMap(w *bufio.Writer, values [][2]string) error {
	if _, err := fmt.Fprintf(w, "%%%d\r\n", len(values)); err != nil {
		return err
	}
	for _, kv := range values {
		if err := writeBulk(w, kv[0]); err != nil {
			return err
		}
		if kv[1] == "" {
			if _, err := w.WriteString("$0\r\n\r\n"); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(kv[1], ":") {
			if _, err := fmt.Fprintf(w, "%s\r\n", kv[1]); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(kv[1], "*") {
			if _, err := w.WriteString(kv[1] + "\r\n"); err != nil {
				return err
			}
			continue
		}
		if err := writeBulk(w, kv[1]); err != nil {
			return err
		}
	}
	return nil
}

func main() {
	ln, err := net.Listen("tcp", "127.0.0.1:6380")
	if err != nil {
		panic(err)
	}
	defer ln.Close()

	st := newStore()
	for {
		conn, err := ln.Accept()
		if err != nil {
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			r := bufio.NewReader(c)
			w := bufio.NewWriter(c)
			for {
				args, err := readCommand(r)
				if err != nil {
					return
				}
				if len(args) == 0 {
					continue
				}
				cmd := strings.ToUpper(args[0])
				switch cmd {
				case "PING":
					if len(args) > 1 {
						_ = writeBulk(w, args[1])
					} else {
						_ = writeSimple(w, "PONG")
					}
				case "ECHO":
					if len(args) > 1 {
						_ = writeBulk(w, args[1])
					} else {
						_ = writeBulk(w, "")
					}
				case "HELLO":
					_ = writeMap(w, [][2]string{
						{"server", "redis"},
						{"version", "7.2.0"},
						{"proto", ":3"},
						{"id", ":1"},
						{"mode", "standalone"},
						{"role", "master"},
						{"modules", "*0"},
					})
				case "SELECT", "CLIENT", "AUTH", "QUIT", "SETNAME", "RESET", "READONLY", "READWRITE":
					_ = writeSimple(w, "OK")
				case "INCR":
					if len(args) < 2 {
						_ = writeError(w, "wrong number of arguments for 'incr'")
						break
					}
					n, err := st.incr(args[1])
					if err != nil {
						_ = writeError(w, err.Error())
						break
					}
					_ = writeInt(w, n)
				case "EXPIRE":
					if len(args) < 3 {
						_ = writeError(w, "wrong number of arguments for 'expire'")
						break
					}
					sec, err := strconv.ParseInt(args[2], 10, 64)
					if err != nil {
						_ = writeError(w, err.Error())
						break
					}
					_ = writeInt(w, st.expire(args[1], sec))
				case "SET":
					if len(args) < 3 {
						_ = writeError(w, "wrong number of arguments for 'set'")
						break
					}
					st.set(args[1], args[2])
					_ = writeSimple(w, "OK")
				case "GET":
					if len(args) < 2 {
						_ = writeError(w, "wrong number of arguments for 'get'")
						break
					}
					if v, ok := st.get(args[1]); ok {
						_ = writeBulk(w, v)
					} else {
						_, _ = w.WriteString("$-1\r\n")
					}
				case "DEL":
					if len(args) < 2 {
						_ = writeError(w, "wrong number of arguments for 'del'")
						break
					}
					_ = writeInt(w, st.del(args[1]))
				case "EXISTS":
					if len(args) < 2 {
						_ = writeError(w, "wrong number of arguments for 'exists'")
						break
					}
					_ = writeInt(w, st.exists(args[1]))
				default:
					_ = writeError(w, "unknown command '"+strings.ToLower(cmd)+"'")
				}
				if err := w.Flush(); err != nil {
					return
				}
			}
		}(conn)
	}
}
