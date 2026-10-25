package controller

import (
	"os/exec"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Quoting", func() {

	Context("quoteSqlIdentifier()", func() {
		It("should wrap an identifier in backticks", func() {
			Expect(quoteSqlIdentifier("stroom")).Should(Equal("`stroom`"))
		})
		It("should escape backticks within an identifier", func() {
			Expect(quoteSqlIdentifier("a`b")).Should(Equal("`a``b`"))
		})
	})

	Context("shellQuote()", func() {
		It("should pass values to the shell literally", func() {
			if _, err := exec.LookPath("bash"); err != nil {
				Skip("bash is required to test shell quoting")
			}
			for _, value := range []string{"stroom", "it's", "$(touch /tmp/x)", "a b; echo c", `"\`} {
				output, err := exec.Command("bash", "-c", "printf '%s' "+shellQuote(value)).Output()
				Expect(err).ToNot(HaveOccurred())
				Expect(string(output)).Should(Equal(value))
			}
		})
	})
})
