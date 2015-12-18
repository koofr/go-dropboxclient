package dropboxclient

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"testing"
)

func TestDropboxclient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dropboxclient Suite")
}
