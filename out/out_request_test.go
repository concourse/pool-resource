package out_test

import (
	"encoding/json"

	. "github.com/ebroberson/pool-resource/out"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("OutRequest", func() {
	var (
		configJSON []byte
	)

	Describe("unmarshalling", func() {
		BeforeEach(func() {
			configJSON = []byte(`{
				"source": {
					"uri": "http://example.com",
					"branch": "develop",
					"private_key": "fake-private-key",
					"pool": "fake-pool",
					"retry_delay": "1h5m10s"
				}
			}`)
		})

		It("parses fields correctly", func() {
			var request OutRequest
			err := json.Unmarshal(configJSON, &request)
			Expect(err).NotTo(HaveOccurred())
			Expect(request.Source.URI).To(Equal("http://example.com"))
			Expect(request.Source.Branch).To(Equal("develop"))
			Expect(request.Source.PrivateKey).To(Equal("fake-private-key"))
			Expect(request.Source.Pool).To(Equal("fake-pool"))
			Expect(request.Source.RetryDelay.String()).To(Equal("1h5m10s"))
		})
	})
})
