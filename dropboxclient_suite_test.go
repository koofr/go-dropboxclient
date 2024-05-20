package dropboxclient

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"testing"
)

func TestDropboxclient(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Dropboxclient Suite")
}
