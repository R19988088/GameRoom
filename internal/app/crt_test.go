package app

import (
	"testing"

	"github.com/egoist/mygo/ui/shader"
)

func TestCRTShaderCompiles(t *testing.T) {
	if _, err := shader.Compile(crtShaderSrc); err != nil {
		t.Fatalf("%v", err)
	}
}
