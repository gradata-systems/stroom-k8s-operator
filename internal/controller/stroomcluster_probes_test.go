package controller

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	stroomv1 "github.com/gradata-systems/stroom-k8s-operator/api/v1"
)

var _ = Describe("StroomCluster configuration file", func() {
	configMount := func(stroomCluster *stroomv1.StroomCluster) corev1.VolumeMount {
		var mounts []corev1.VolumeMount
		(&StroomClusterReconciler{}).appendConfigVolumeMounts(stroomCluster, &mounts)
		Expect(mounts).To(HaveLen(1))
		return mounts[0]
	}
	https := stroomv1.HttpsSettings{
		TlsSecretName:                "stroom-tls",
		TlsKeystorePasswordSecretRef: stroomv1.SecretItem{SecretName: "stroom-keystore", Key: "password"},
	}

	It("should mount the bundled configuration without an HTTPS connector, when HTTPS isn't enabled", func() {
		Expect(configMount(&stroomv1.StroomCluster{}).SubPath).To(Equal("stroomcluster-config.yaml"))
	})
	It("should mount the bundled configuration with an HTTPS connector, when HTTPS is enabled", func() {
		stroomCluster := &stroomv1.StroomCluster{}
		stroomCluster.Spec.Https = https
		Expect(configMount(stroomCluster).SubPath).To(Equal("stroomcluster-config-https.yaml"))
	})
	It("should mount the referenced configuration, with or without HTTPS", func() {
		stroomCluster := &stroomv1.StroomCluster{}
		stroomCluster.Spec.ConfigMapRef = stroomv1.ConfigMapRef{Name: "stroom-config", ItemName: "config.yml"}
		stroomCluster.Spec.Https = https
		Expect(configMount(stroomCluster)).To(Equal(corev1.VolumeMount{
			Name: "config-override", SubPath: "config.yml", MountPath: "/stroom/config/config.yml", ReadOnly: true,
		}))
	})
})

var _ = Describe("StroomCluster probes", func() {

	Context("healthCheckPath()", func() {
		It("should run all health checks when no filters are provided", func() {
			Expect(healthCheckPath(nil, nil)).Should(Equal("/stroomAdmin/filteredhealthcheck"))
		})
		It("should pass the allow list as a single comma-delimited parameter", func() {
			Expect(healthCheckPath([]string{"deadlocks", "stroom.config.global.impl.AppConfigMonitor"}, nil)).
				Should(Equal("/stroomAdmin/filteredhealthcheck?allow=deadlocks%2Cstroom.config.global.impl.AppConfigMonitor"))
		})
		It("should pass the deny list", func() {
			Expect(healthCheckPath(nil, []string{"stroom.security.common.impl.ExternalIdpConfigurationProvider"})).
				Should(Equal("/stroomAdmin/filteredhealthcheck?deny=stroom.security.common.impl.ExternalIdpConfigurationProvider"))
		})
		It("should pass both lists", func() {
			Expect(healthCheckPath([]string{"deadlocks"}, []string{"b"})).
				Should(Equal("/stroomAdmin/filteredhealthcheck?allow=deadlocks&deny=b"))
		})
		It("should trim names and omit blank names", func() {
			Expect(healthCheckPath([]string{" deadlocks ", "", "  "}, []string{" "})).
				Should(Equal("/stroomAdmin/filteredhealthcheck?allow=deadlocks"))
		})
		It("should run all health checks when lists are explicitly empty", func() {
			Expect(healthCheckPath([]string{}, []string{})).Should(Equal("/stroomAdmin/filteredhealthcheck"))
		})
	})

	Context("createHealthCheckProbe()", func() {
		timings := stroomv1.ProbeTimings{
			InitialDelaySeconds: 1,
			TimeoutSeconds:      2,
			PeriodSeconds:       3,
			SuccessThreshold:    4,
			FailureThreshold:    5,
		}

		It("should apply each probe timing to the matching probe field", func() {
			probe := createHealthCheckProbe(&timings, nil, nil, AdminPortName)
			Expect(probe.InitialDelaySeconds).Should(Equal(int32(1)))
			Expect(probe.TimeoutSeconds).Should(Equal(int32(2)))
			Expect(probe.PeriodSeconds).Should(Equal(int32(3)))
			Expect(probe.SuccessThreshold).Should(Equal(int32(4)))
			Expect(probe.FailureThreshold).Should(Equal(int32(5)))
		})
		It("should query the admin port with the filtered health check path", func() {
			probe := createHealthCheckProbe(&timings, []string{"deadlocks"}, nil, AdminPortName)
			Expect(probe.HTTPGet.Path).Should(Equal("/stroomAdmin/filteredhealthcheck?allow=deadlocks"))
			Expect(probe.HTTPGet.Port.StrVal).Should(Equal(AdminPortName))
		})
	})

	Context("createStartupProbe()", func() {
		newCluster := func(policy appsv1.PodManagementPolicyType) *stroomv1.StroomCluster {
			return &stroomv1.StroomCluster{Spec: stroomv1.StroomClusterSpec{PodManagementPolicy: policy}}
		}
		newNodeSet := func(count int32, periodSeconds int32, failureThreshold int32, allowanceSeconds int32) *stroomv1.NodeSet {
			return &stroomv1.NodeSet{
				Count: count,
				StartupProbeTimings: stroomv1.StartupProbeTimings{
					InitialDelaySeconds:      5,
					TimeoutSeconds:           5,
					PeriodSeconds:            periodSeconds,
					SuccessThreshold:         1,
					FailureThreshold:         failureThreshold,
					LockWaitAllowanceSeconds: allowanceSeconds,
				},
			}
		}

		It("should run the node startup script", func() {
			probe := createStartupProbe(newCluster(appsv1.OrderedReadyPodManagement), newNodeSet(1, 5, 24, 30))
			Expect(probe.Exec.Command).Should(Equal([]string{"bash", "/stroom/scripts/node-start.sh"}))
			Expect(probe.PeriodSeconds).Should(Equal(int32(5)))
		})
		It("should not add a lock wait allowance when pods start one at a time", func() {
			probe := createStartupProbe(newCluster(appsv1.OrderedReadyPodManagement), newNodeSet(3, 5, 24, 30))
			Expect(probe.FailureThreshold).Should(Equal(int32(24)))
		})
		It("should not add a lock wait allowance when the policy is unset", func() {
			probe := createStartupProbe(newCluster(""), newNodeSet(3, 5, 24, 30))
			Expect(probe.FailureThreshold).Should(Equal(int32(24)))
		})
		It("should not add a lock wait allowance for a single node", func() {
			probe := createStartupProbe(newCluster(appsv1.ParallelPodManagement), newNodeSet(1, 5, 24, 30))
			Expect(probe.FailureThreshold).Should(Equal(int32(24)))
		})
		It("should add the allowance for each additional node when pods start in parallel", func() {
			// 30 seconds x 2 additional nodes / 5 second period = 12 additional failures
			probe := createStartupProbe(newCluster(appsv1.ParallelPodManagement), newNodeSet(3, 5, 24, 30))
			Expect(probe.FailureThreshold).Should(Equal(int32(36)))
		})
		It("should round the allowance up to a whole probe period", func() {
			// 7 seconds x 1 additional node / 5 second period = 1.4, rounded up to 2
			probe := createStartupProbe(newCluster(appsv1.ParallelPodManagement), newNodeSet(2, 5, 24, 7))
			Expect(probe.FailureThreshold).Should(Equal(int32(26)))
		})
		It("should not add an allowance when it is zero", func() {
			probe := createStartupProbe(newCluster(appsv1.ParallelPodManagement), newNodeSet(3, 5, 24, 0))
			Expect(probe.FailureThreshold).Should(Equal(int32(24)))
		})
		It("should use the Kubernetes default period and threshold when they are unset", func() {
			// 3 (default threshold) + 30 seconds x 1 additional node / 10 second (default) period
			probe := createStartupProbe(newCluster(appsv1.ParallelPodManagement), newNodeSet(2, 0, 0, 30))
			Expect(probe.FailureThreshold).Should(Equal(int32(6)))
		})
	})
})
