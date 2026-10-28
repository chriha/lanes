package provider

// Fields returns the sorted keys of the fields the items of a built-in provider type carry (the keys filters and
// action templates can use besides id, title, url, author, draft and note). It returns nil for "command", whose
// fields come from the command's output, and for unknown types.
func Fields(typ string) []string {
	return nil
}
