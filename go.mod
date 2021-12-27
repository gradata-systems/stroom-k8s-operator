module github.com/gradata-systems/stroom-k8s-operator

go 1.16

require (
	github.com/go-logr/logr v0.4.0
	github.com/go-sql-driver/mysql v1.6.0
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.15.0
	github.com/sethvargo/go-password v0.2.0
	k8s.io/api v0.22.1
	k8s.io/apimachinery v0.22.1
	k8s.io/client-go v0.22.1
	k8s.io/metrics v0.22.1
	sigs.k8s.io/controller-runtime v0.10.0
)
