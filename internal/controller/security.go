package controller

import (
	corev1 "k8s.io/api/core/v1"
)

// The pods the operator creates don't use the Kubernetes API, so don't need a service account token
var automountServiceAccountToken = false

// containerSecurityContext returns a container's security context, with privilege escalation disallowed unless the
// user has set it, or set something that requires it. Capabilities and the user aren't changed, as the Stroom and
// MySQL images start as root to set up their volumes, then run as an unprivileged user.
func containerSecurityContext(securityContext corev1.SecurityContext) *corev1.SecurityContext {
	result := securityContext.DeepCopy()
	if result.AllowPrivilegeEscalation == nil && !requiresPrivilegeEscalation(result) {
		allowPrivilegeEscalation := false
		result.AllowPrivilegeEscalation = &allowPrivilegeEscalation
	}
	return result
}

// requiresPrivilegeEscalation returns whether a security context has settings that Kubernetes only accepts with
// privilege escalation allowed
func requiresPrivilegeEscalation(securityContext *corev1.SecurityContext) bool {
	if securityContext.Privileged != nil && *securityContext.Privileged {
		return true
	}
	if securityContext.Capabilities != nil {
		for _, capability := range securityContext.Capabilities.Add {
			if capability == "SYS_ADMIN" || capability == "CAP_SYS_ADMIN" {
				return true
			}
		}
	}
	return false
}

// podSecurityContext returns a pod's security context, with the container runtime's default seccomp profile unless
// the user has set a profile
func podSecurityContext(securityContext corev1.PodSecurityContext) *corev1.PodSecurityContext {
	result := securityContext.DeepCopy()
	if result.SeccompProfile == nil {
		result.SeccompProfile = &corev1.SeccompProfile{Type: corev1.SeccompProfileTypeRuntimeDefault}
	}
	return result
}
