// The basics of Go in one small program (chapter 4.1):
// variables, functions, if, for, and reading input.
//
//	go run ./examples/basics
//	go run ./examples/basics 7
package main

import (
	"fmt"
	"os"
	"strconv"
)

// shortCodeLength is a constant: a value that never changes.
const shortCodeLength = 7

func main() {
	// Variables: a name for a value. := declares and assigns in one go.
	name := "snip"
	links := 3
	fmt.Println("Hello from", name, "- links so far:", links)

	// A function call: give it inputs, get an output back.
	total := combinations(56, shortCodeLength)
	fmt.Printf("%d-character codes from 56 symbols: %d combinations\n", shortCodeLength, total)

	// Reading a command-line argument, and handling the error if it's not a number.
	n := 5
	if len(os.Args) > 1 {
		parsed, err := strconv.Atoi(os.Args[1])
		if err != nil {
			fmt.Println("please give a whole number, got:", os.Args[1])
			os.Exit(1)
		}
		n = parsed
	}

	// A loop that repeats n times, with an if inside it.
	for i := 1; i <= n; i++ {
		if i%2 == 0 {
			fmt.Println(i, "is even")
		} else {
			fmt.Println(i, "is odd")
		}
	}
}

// combinations returns symbols multiplied by itself length times.
func combinations(symbols, length int) int {
	result := 1
	for i := 0; i < length; i++ {
		result *= symbols
	}
	return result
}
