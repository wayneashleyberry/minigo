package strngs

func HasSuffix(s string, suffix string) bool {
	if len(s) >= len(suffix) {
		var suf string
		suf = s[len(s)-len(suffix):]
		return suf == suffix
	}
	return false
}

// Contains reports whether substr is within s.
func Contains(s string, substr string) bool {
	return Index(s, substr) >= 0
}

func Index(s string, substr string) int {
	bytes := []byte(s)
	bsub := []byte(substr)
	var in bool
	var subIndex int
	var r int = -1 // not found
	for i, b := range bytes {
		if !in && b == bsub[0] {
			in = true
			r = i
			subIndex = 0
		}

		if in {
			if b == bsub[subIndex] {
				if subIndex == len(bsub) - 1 {
					return r
				}
			} else {
				in = false
				r = -1
				subIndex = 0
			}
		}
	}

	return -1
}

// "foo/bar", "/" => []string{"foo", "bar"}
func Split(s string, sep string) []string {
	if len(sep) > 1  {
		panic("no supported")
	}
	seps := []byte(sep)
	sepchar := seps[0]
	bytes := []byte(s)
	var buf []byte
	var r []string
	for _, b := range bytes {
		if b == sepchar {
			r = append(r, string(buf))
			buf = nil
		} else {
			buf = append(buf, b)
		}
	}
	r = append(r, string(buf))

	return r
}
