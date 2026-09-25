// Command breached checks passwords against the public Pwned Passwords range
// API with k-anonymity (chapter 7.8): only the first 5 characters of each
// password's SHA-1 hash are sent.
//
//	go run ./examples/accountsec/cmd 'password123' 'correct horse battery staple'
package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/jawwadzafar/zero-to-prod/snip/examples/accountsec"
)

func main() {
	client := &http.Client{Timeout: 10 * time.Second}
	for _, pw := range os.Args[1:] {
		n, err := accountsec.BreachCount(context.Background(), client, accountsec.RangeURL, pw)
		if err != nil {
			fmt.Fprintln(os.Stderr, "error:", err)
			os.Exit(1)
		}
		fmt.Printf("%-32q seen in breaches %d times\n", pw, n)
	}
}
