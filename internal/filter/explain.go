package filter

// Split cuts src at its top-level `&&`, `||` or `!` and returns that operator ("" when src has none) and the
// operands' source.
func Split(src string) (op string, operands []string, err error) {
	return "", nil, nil
}

// Failed returns the top-level operands that made f not match env; it is nil when f matches.
func Failed(src string, env map[string]any) ([]string, error) {
	return nil, nil
}
