package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
)

var _ = Describe("Security defaults", func() {
	boolPtr := func(value bool) *bool { return &value }

	Context("containerSecurityContext()", func() {
		It("should disallow privilege escalation by default", func() {
			Expect(containerSecurityContext(corev1.SecurityContext{}).AllowPrivilegeEscalation).To(HaveValue(BeFalse()))
		})
		It("should keep the user's settings", func() {
			result := containerSecurityContext(corev1.SecurityContext{
				AllowPrivilegeEscalation: boolPtr(true),
				RunAsNonRoot:             boolPtr(true),
			})
			Expect(result.AllowPrivilegeEscalation).To(HaveValue(BeTrue()))
			Expect(result.RunAsNonRoot).To(HaveValue(BeTrue()))
		})
		It("should leave privilege escalation unset for settings that require it", func() {
			Expect(containerSecurityContext(corev1.SecurityContext{Privileged: boolPtr(true)}).AllowPrivilegeEscalation).To(BeNil())
			withSysAdmin := corev1.SecurityContext{Capabilities: &corev1.Capabilities{Add: []corev1.Capability{"SYS_ADMIN"}}}
			Expect(containerSecurityContext(withSysAdmin).AllowPrivilegeEscalation).To(BeNil())
		})
		It("should not change the user's security context", func() {
			securityContext := corev1.SecurityContext{}
			containerSecurityContext(securityContext)
			Expect(securityContext.AllowPrivilegeEscalation).To(BeNil())
		})
	})

	Context("podSecurityContext()", func() {
		It("should use the runtime's default seccomp profile by default", func() {
			Expect(podSecurityContext(corev1.PodSecurityContext{}).SeccompProfile).To(Equal(
				&corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}))
		})
		It("should keep the user's seccomp profile", func() {
			unconfined := &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeUnconfined}
			Expect(podSecurityContext(corev1.PodSecurityContext{SeccompProfile: unconfined}).SeccompProfile).To(Equal(unconfined))
		})
	})
})
