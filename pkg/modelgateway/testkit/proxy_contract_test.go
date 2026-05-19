package testkit_test

import (
	"path/filepath"
	"testing"

	"github.com/QuantumNous/new-api/pkg/modelgateway/testkit"
)

func TestProxyContracts(t *testing.T) {
	for _, path := range testkit.ProxyContractPaths(t) {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			testkit.RunProxyContract(t, path)
		})
	}
}
