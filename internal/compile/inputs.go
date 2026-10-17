package compile

// Called by concurrent esbuild loaders. Preview watches resolved dependencies;
// it never needs to crawl an entire node_modules tree on each edit.
func (b *builder) recordInput(path string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.inputs == nil {
		b.inputs = map[string]bool{}
	}
	b.inputs[path] = true
}
