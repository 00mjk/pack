package name

import (
	"fmt"

	gname "github.com/google/go-containerregistry/pkg/name"
)

func TranslateRegistry(name string, registryMirrors map[string]string) (string, error) {
	if registryMirrors == nil {
		return name, nil
	}

	srcRef, err := gname.ParseReference(name, gname.WeakValidation)
	if err != nil {
		return "", err
	}

	srcContext := srcRef.Context()
	registryMirror, ok := registryMirrors[srcContext.RegistryStr()]
	if !ok {
		return name, nil
	}

	refName := fmt.Sprintf("%s/%s:%s", registryMirror, srcContext.RepositoryStr(), srcRef.Identifier())
	_, err = gname.ParseReference(refName, gname.WeakValidation)
	if err != nil {
		return "", err
	}

	return refName, nil
}
