package scanner

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// Clamd talks to a ClamAV daemon over its TCP protocol.
type Clamd struct {
	Addr string
	// Timeout bounds one whole operation (dial + stream + verdict).
	Timeout time.Duration
}

// Enabled reports whether a clamd endpoint is configured.
func (c *Clamd) Enabled() bool { return c != nil && c.Addr != "" }

func (c *Clamd) timeout() time.Duration {
	if c.Timeout > 0 {
		return c.Timeout
	}
	return 30 * time.Second
}

func (c *Clamd) dial(ctx context.Context) (net.Conn, error) {
	d := net.Dialer{Timeout: c.timeout()}
	conn, err := d.DialContext(ctx, "tcp", c.Addr)
	if err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Now().Add(c.timeout()))
	return conn, nil
}

// Ping checks that clamd is up AND ready: clamd only answers once its
// signature database is loaded, which is exactly the alive-vs-ready
// distinction §115 asks for.
func (c *Clamd) Ping(ctx context.Context) error {
	conn, err := c.dial(ctx)
	if err != nil {
		return fmt.Errorf("clamd unreachable (down, or still loading signatures): %w", err)
	}
	defer conn.Close()
	if _, err := io.WriteString(conn, "zPING\x00"); err != nil {
		return err
	}
	reply, err := bufio.NewReader(conn).ReadString('\x00')
	if err != nil && !strings.Contains(reply, "PONG") {
		return fmt.Errorf("clamd ping failed: %w", err)
	}
	if !strings.Contains(reply, "PONG") {
		return fmt.Errorf("clamd ping answered %q", strings.Trim(reply, "\x00"))
	}
	return nil
}

// ScanResult is one malware-scan verdict.
type ScanResult struct {
	Infected bool
	// Virus is the signature name when Infected.
	Virus string
}

// Scan streams content through clamd INSTREAM and returns the verdict
// (§45: content is scanned, never judged by extension).
func (c *Clamd) Scan(ctx context.Context, content io.Reader) (*ScanResult, error) {
	conn, err := c.dial(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err := io.WriteString(conn, "zINSTREAM\x00"); err != nil {
		return nil, err
	}
	buf := make([]byte, 32<<10)
	var size [4]byte
	for {
		n, rerr := content.Read(buf)
		if n > 0 {
			binary.BigEndian.PutUint32(size[:], uint32(n))
			if _, err := conn.Write(size[:]); err != nil {
				return nil, err
			}
			if _, err := conn.Write(buf[:n]); err != nil {
				return nil, err
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return nil, rerr
		}
	}
	binary.BigEndian.PutUint32(size[:], 0)
	if _, err := conn.Write(size[:]); err != nil {
		return nil, err
	}
	reply, err := bufio.NewReader(conn).ReadString('\x00')
	if err != nil && reply == "" {
		return nil, err
	}
	reply = strings.Trim(strings.TrimPrefix(reply, "stream: "), "\x00 \r\n")
	switch {
	case reply == "OK":
		return &ScanResult{}, nil
	case strings.HasSuffix(reply, "FOUND"):
		return &ScanResult{Infected: true, Virus: strings.TrimSpace(strings.TrimSuffix(reply, "FOUND"))}, nil
	default:
		return nil, fmt.Errorf("clamd scan answered %q", reply)
	}
}
