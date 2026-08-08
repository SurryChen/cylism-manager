package runtime

import "strings"

// ImageVersion extracts a meaningful version tag from a container image
// reference. Digest references, registry ports and the "latest" tag carry no
// version information and yield an empty string.
func ImageVersion(image string) string {
	image = strings.TrimSpace(image)
	if image == "" {
		return ""
	}
	if strings.Contains(image, "@") {
		return ""
	}
	index := strings.LastIndex(image, ":")
	if index < 0 || index == len(image)-1 {
		return ""
	}
	tag := image[index+1:]
	if tag == "" || strings.Contains(tag, "/") {
		// The colon belongs to a registry port (registry:5000/img), not a tag.
		return ""
	}
	if tag == "latest" {
		return ""
	}
	return tag
}
