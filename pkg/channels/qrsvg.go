package channels

import (
	"fmt"
	"strings"

	"rsc.io/qr"
)

// generateQRSVG produces a self-contained SVG string for the given QR data.
// The SVG uses a white background with black modules, suitable for embedding
// directly in an HTML <img> tag or innerHTML.
func generateQRSVG(data string, size int) (string, error) {
	code, err := qr.Encode(data, qr.L)
	if err != nil {
		return "", fmt.Errorf("failed to encode QR: %w", err)
	}

	n := code.Size
	if n == 0 {
		return "", fmt.Errorf("empty QR code")
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf(
		`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="%d" height="%d">`,
		n, n, size, size,
	))
	sb.WriteString(fmt.Sprintf(`<rect width="%d" height="%d" fill="#fff"/>`, n, n))

	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			if code.Black(x, y) {
				sb.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="1" height="1" fill="#000"/>`, x, y))
			}
		}
	}

	sb.WriteString(`</svg>`)
	return sb.String(), nil
}
