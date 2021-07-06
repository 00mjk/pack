package name_test

import (
	"testing"

	"github.com/sclevine/spec"
	"github.com/sclevine/spec/report"

	"github.com/buildpacks/pack/internal/name"
	h "github.com/buildpacks/pack/testhelpers"
)

func TestTranslateRegistry(t *testing.T) {
	spec.Run(t, "TranslateRegistry", testTranslateRegistry, spec.Report(report.Terminal{}))
}

func testTranslateRegistry(t *testing.T, when spec.G, it spec.S) {
	var (
		assert = h.NewAssertionManager(t)
	)

	when("#TranslateRegistry", func() {
		it("doesn't translate when there are no mirrors", func() {
			input := "index.docker.io/my/buildpack:0.1"

			output, err := name.TranslateRegistry(input, nil)
			assert.Nil(err)
			assert.Equal(output, input)
		})

		it("doesn't translate when there are is no matching mirrors", func() {
			input := "index.docker.io/my/buildpack:0.1"
			registryMirrors := map[string]string{
				"us.gcr.io": "10.0.0.1",
			}

			output, err := name.TranslateRegistry(input, registryMirrors)
			assert.Nil(err)
			assert.Equal(output, input)
		})

		it("translates when there is a mirror", func() {
			input := "index.docker.io/my/buildpack:0.1"
			expected := "10.0.0.1/my/buildpack:0.1"
			registryMirrors := map[string]string{
				"index.docker.io": "10.0.0.1",
			}

			output, err := name.TranslateRegistry(input, registryMirrors)
			assert.Nil(err)
			assert.Equal(output, expected)
		})
	})
}
