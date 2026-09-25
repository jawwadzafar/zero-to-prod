// Command totp shows two-factor codes working end to end (chapter 7.7):
//
//	go run ./examples/totp/cmd enroll          # new secret + otpauth:// link for your app
//	go run ./examples/totp/cmd code SECRET      # what your app shows right now
//	go run ./examples/totp/cmd verify SECRET 123456
package main

import (
	"encoding/base32"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jawwadzafar/zero-to-prod/snip/examples/totp"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "usage: enroll | code SECRET | verify SECRET CODE")
		os.Exit(2)
	}
	switch os.Args[1] {
	case "enroll":
		secret, err := totp.NewSecret()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Println("secret:", totp.Base32(secret))
		fmt.Println("link:  ", totp.URI(secret, "snip", "you@example.com"))
		fmt.Println("Turn the link into a QR code, or type the secret into your authenticator app.")
	case "code", "verify":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "missing SECRET")
			os.Exit(2)
		}
		secret, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(os.Args[2]))
		if err != nil {
			fmt.Fprintln(os.Stderr, "secret is not valid base32:", err)
			os.Exit(1)
		}
		now := time.Now()
		if os.Args[1] == "code" {
			left := totp.Step - time.Duration(now.Unix()%int64(totp.Step.Seconds()))*time.Second
			fmt.Printf("%s  (changes in %ds)\n", totp.TOTP(secret, now, 6), int(left.Seconds()))
			return
		}
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "missing CODE")
			os.Exit(2)
		}
		v := &totp.Verifier{Secret: secret, Skew: 1}
		if v.Verify(os.Args[3], now) {
			fmt.Println("ok: code accepted")
			return
		}
		fmt.Println("rejected")
		os.Exit(1)
	default:
		fmt.Fprintln(os.Stderr, "unknown command", os.Args[1])
		os.Exit(2)
	}
}
