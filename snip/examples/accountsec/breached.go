package accountsec

import (
	"bufio"
	"context"
	"crypto/sha1" //nolint:gosec // the breached-password range API is keyed by SHA-1; nothing here relies on SHA-1 for security
	"encoding/hex"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// RangeURL is the public "Pwned Passwords" range API: it takes the first five
// hex characters of a password's SHA-1 hash and returns every leaked hash
// that starts with them, with how often each was seen.
const RangeURL = "https://api.pwnedpasswords.com/range/"

// BreachCount reports how many times a password appears in known breaches,
// using k-anonymity: only 5 of the hash's 40 characters leave this machine,
// and hundreds of different hashes share any given prefix, so the service
// never learns which password was checked.
func BreachCount(ctx context.Context, client *http.Client, baseURL, password string) (int, error) {
	sum := sha1.Sum([]byte(password)) //nolint:gosec // see import comment
	full := strings.ToUpper(hex.EncodeToString(sum[:]))
	prefix, suffix := full[:5], full[5:]

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+prefix, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Add-Padding", "true") // pad responses so their size doesn't leak the prefix's popularity
	res, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("range API: %s", res.Status)
	}
	scanner := bufio.NewScanner(res.Body)
	for scanner.Scan() {
		// Each line is "SUFFIX:COUNT". Padding lines have a count of 0.
		hashSuffix, count, ok := strings.Cut(strings.TrimSpace(scanner.Text()), ":")
		if ok && hashSuffix == suffix {
			return strconv.Atoi(count)
		}
	}
	return 0, scanner.Err()
}
