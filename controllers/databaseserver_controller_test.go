package controllers

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"time"
)

var _ = Describe("CronJob controller", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		CronjobName      = "test-cronjob"
		CronjobNamespace = "default"
		JobName          = "test-job"

		timeout  = time.Second * 10
		duration = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When updating CronJob Status", func() {
		It("Should return true", func() {
			By("Being true")
			Expect(true).Should(Equal(true))

			// Create
			//Expect(k8sClient.Create(context.Background(), created)).Should(Succeed())
			//
			//By("Expecting submitted")
			//Eventually(func() bool {
			//	f := &stroomv1.DatabaseServer{}
			//	k8sClient.Get(context.Background(), key, f)
			//	return f.IsSubmitted()
			//}, timeout, interval).Should(BeTrue())
		})
	})
})
