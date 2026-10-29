// Command presign prints a presigned URL for one object (chapter 6.10).
//
//	export AWS_ACCESS_KEY_ID=admin AWS_SECRET_ACCESS_KEY=secret123
//	go run ./examples/presign/cmd/presign -method PUT -bucket uploads -key cat.png
//
// It signs path-style URLs (endpoint/bucket/key), which every S3-compatible
// server accepts. The result works for -expires, then the server refuses it.
package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jawwadzafar/zero-to-prod/snip/examples/presign"
)

func main() {
	method := flag.String("method", "GET", "HTTP method the URL allows (GET to download, PUT to upload)")
	endpoint := flag.String("endpoint", "http://localhost:8333", "S3 endpoint")
	bucket := flag.String("bucket", "uploads", "bucket name")
	key := flag.String("key", "", "object key, e.g. avatars/42.png")
	expires := flag.Duration("expires", 15*time.Minute, "how long the URL stays valid (max 168h)")
	flag.Parse()
	if *key == "" {
		fmt.Fprintln(os.Stderr, "usage: presign -key <object key> [-method PUT] [-bucket b] [-endpoint url]")
		os.Exit(2)
	}

	region := os.Getenv("AWS_REGION")
	if region == "" {
		region = "us-east-1"
	}
	creds := presign.Credentials{
		AccessKeyID:     os.Getenv("AWS_ACCESS_KEY_ID"),
		SecretAccessKey: os.Getenv("AWS_SECRET_ACCESS_KEY"),
	}

	// Escape each path segment so keys with spaces or unicode sign correctly.
	segs := strings.Split(*key, "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	raw := strings.TrimRight(*endpoint, "/") + "/" + url.PathEscape(*bucket) + "/" + strings.Join(segs, "/")

	signed, err := presign.URL(strings.ToUpper(*method), raw, creds, region, time.Now(), *expires)
	if err != nil {
		fmt.Fprintln(os.Stderr, "presign:", err)
		os.Exit(1)
	}
	fmt.Println(signed)
}
