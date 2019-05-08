package main

import (
	"fmt"
)

var GENERATION int = 2

var debugMode = true
var debugToken = false

func f3() {
	path := "t/min/min.go"
	s := readFile(path)
	_bs := ByteStream{
		filename:  path,
		source:    s,
		nextIndex: 0,
		line:      1,
		column:    0,
	}
	bs = &_bs

	var c byte
	c, _ = bs.get()
	ident := readIdentifier(c)
	fmt.Printf("%s\n", ident)
}

func f4() {
	path := "t/min/min.go"
	s := readFile(path)
	_bs := ByteStream{
		filename:  path,
		source:    s,
		nextIndex: 0,
		line:      1,
		column:    0,
	}
	bs = &_bs

	tokens := tokenize(bs)
	fmt.Printf("%d\n", len(tokens)) // 26
	fmt.Printf("----------\n")
	for _, tok := range tokens {
		fmt.Printf("%s:%s\n", string(tok.typ), tok.sval)
	}
}

func f5() {
	debugToken = false
	path := "t/data/string.txt"
	s := readFile(path)
	_bs := ByteStream{
		filename:  path,
		source:    s,
		nextIndex: 0,
		line:      1,
		column:    0,
	}
	bs = &_bs

	tokens := tokenize(bs)
	tok := tokens[0]
	fmt.Printf("----------\n")
	fmt.Printf("[%s]\n", tok.sval)
}

func main() {
	f3()
	f4()
	f5()
}
