// Command dkimverify cryptographically verifies the DKIM signature of a raw
// RFC822 message read from stdin (V4 §27/§122: header presence is not
// proof). The public key is supplied on the command line and served to the
// verifier through an in-process DNS override, so the check needs no real
// DNS — suitable for controlled E2E environments.
//
//	docker exec mailplatform-smtpsink cat /data/sink.jsonl | ... |
//	  go run ./cmd/dkimverify -domain company.test -selector s1rsa -pubkey <base64>
//
// Exit code 0: at least one signature by -domain verified cryptographically.
// Exit code 1: no valid signature (missing, wrong key, or body tampered).
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/emersion/go-msgauth/dkim"
)

func main() {
	domain := flag.String("domain", "", "expected signing domain (d= tag)")
	selector := flag.String("selector", "", "expected selector (s= tag)")
	pubkey := flag.String("pubkey", "", "base64 public key the DNS record would carry (p= value)")
	keyType := flag.String("k", "rsa", "key type for the synthesized DNS record (rsa | ed25519)")
	flag.Parse()
	if *domain == "" || *selector == "" || *pubkey == "" {
		fmt.Fprintln(os.Stderr, "dkimverify: -domain, -selector and -pubkey are required")
		os.Exit(2)
	}

	record := fmt.Sprintf("v=DKIM1; k=%s; p=%s", *keyType, *pubkey)
	wantHost := *selector + "._domainkey." + *domain

	opts := &dkim.VerifyOptions{
		LookupTXT: func(name string) ([]string, error) {
			if name == wantHost {
				return []string{record}, nil
			}
			return nil, fmt.Errorf("no TXT fixture for %q (expected %q)", name, wantHost)
		},
	}
	verifications, err := dkim.VerifyWithOptions(os.Stdin, opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "dkimverify: %v\n", err)
		os.Exit(1)
	}
	ok := false
	for _, v := range verifications {
		status := "FAIL"
		if v.Err == nil {
			status = "PASS"
		}
		fmt.Printf("%s d=%s s=%s", status, v.Domain, "(see header)")
		if v.Err != nil {
			fmt.Printf(" err=%v", v.Err)
		}
		fmt.Println()
		if v.Err == nil && v.Domain == *domain {
			ok = true
		}
	}
	if !ok {
		fmt.Fprintln(os.Stderr, "dkimverify: no cryptographically valid signature for domain", *domain)
		os.Exit(1)
	}
	fmt.Println("signature cryptographically valid")
}
