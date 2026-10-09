package semanticreview

// ValidateCurationSource makes the exact PIT and content-hash shape used by
// FreezeCuration available to upstream archive importers. This validates
// supplied metadata, not the authenticity of an external evidence archive.
func ValidateCurationSource(src CurationSource) error {
	return validateSource(src)
}
