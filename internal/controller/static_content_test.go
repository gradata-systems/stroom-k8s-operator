package controller

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// fakeCurl stands in for curl in the node lifecycle scripts. It records each request URL, its arguments and its
// request body, and replies to task list requests with the responses listed in the task-list-responses file, one
// per call, as "<http code> <body>".
const fakeCurl = `#!/bin/bash
url=''
for arg in "$@"; do
  if [[ $arg == http* ]]; then url=$arg; fi
done
echo "$url" >> "$FAKE_CURL_DIR/requests"
echo "$*" >> "$FAKE_CURL_DIR/arguments"

case "$url" in
  */authproxy/v1/fetchClientCredsToken)
    cat > "$FAKE_CURL_DIR/token-request-body"
    printf 'header.eyJleHAiOjk5OTk5OTk5OTl9.signature\nhttp_code=200' ;;
  */task/v1/list/*)
    call=$(grep -c '/task/v1/list/' "$FAKE_CURL_DIR/requests")
    read -r code body <<< "$(sed -n "${call}p" "$FAKE_CURL_DIR/task-list-responses")"
    printf '%s\nhttp_code=%s' "$body" "$code" ;;
  *)
    printf '{}\nhttp_code=200' ;;
esac
`

var _ = Describe("Bundled Stroom configuration", func() {
	It("should only add the HTTPS connector, in the HTTPS variant", func() {
		read := func(file string) string {
			content, err := StaticFiles.ReadFile("static_content/" + file)
			Expect(err).ToNot(HaveOccurred())
			return strings.ReplaceAll(string(content), "\r\n", "\n")
		}
		// Header comments that say which variant is which
		withoutComments := func(content string) string {
			var lines []string
			for _, line := range strings.Split(content, "\n") {
				if !strings.HasPrefix(line, "# The operator uses this file") && !strings.HasPrefix(line, "# the HTTPS connector") {
					lines = append(lines, line)
				}
			}
			return strings.Join(lines, "\n")
		}
		const httpsConnector = `    - type: https
      port: ${STROOM_APP_HTTPS_PORT:-8443}
      keyStoreType: PKCS12
      keyStorePath: /stroom/pki/tls/keystore.p12
      keyStorePassword: ${STROOM_KEYSTORE_PASSWORD}
      trustStoreType: PKCS12
      trustStorePath: /stroom/pki/tls/truststore.p12
      trustStorePassword: ${STROOM_KEYSTORE_PASSWORD}
      validateCerts: false
`
		http := withoutComments(read("stroomcluster-config.yaml"))
		https := withoutComments(read("stroomcluster-config-https.yaml"))
		Expect(http).ToNot(ContainSubstring("type: https"))
		Expect(https).To(ContainSubstring(httpsConnector))
		Expect(strings.Replace(https, httpsConnector, "", 1)).To(Equal(http))
	})
})

var _ = Describe("generate-keystore.sh", func() {
	It("should create a key store and trust store readable only by the user Stroom runs as", func() {
		for _, tool := range []string{"bash", "openssl"} {
			if _, err := exec.LookPath(tool); err != nil {
				Skip(tool + " is required to test the key store script")
			}
		}
		if os.Getuid() == 0 {
			Skip("the script gives the files to the stroom user when run as root, which this test can't check")
		}
		// The trust store needs -jdktrust, from OpenSSL 3.2, which the Stroom image has. The e2e tests cover the
		// script where the local OpenSSL is older.
		if help, _ := exec.Command("openssl", "pkcs12", "-help").CombinedOutput(); !strings.Contains(string(help), "-jdktrust") {
			Skip("the key store script needs OpenSSL 3.2 or later, for -jdktrust")
		}

		root := GinkgoT().TempDir()
		tlsDir := filepath.Join(root, "tls")
		keystoreDir := filepath.Join(root, "keystore")
		Expect(os.MkdirAll(tlsDir, 0o755)).To(Succeed())
		Expect(os.MkdirAll(keystoreDir, 0o755)).To(Succeed())
		certificate := exec.Command("openssl", "req", "-x509", "-newkey", "rsa:2048", "-nodes", "-days", "1",
			"-subj", "/CN=stroom", "-keyout", filepath.Join(tlsDir, "tls.key"), "-out", filepath.Join(tlsDir, "tls.crt"))
		output, err := certificate.CombinedOutput()
		Expect(err).ToNot(HaveOccurred(), string(output))
		content, err := os.ReadFile(filepath.Join(tlsDir, "tls.crt"))
		Expect(err).ToNot(HaveOccurred())
		Expect(os.WriteFile(filepath.Join(tlsDir, "ca.crt"), content, 0o644)).To(Succeed())

		script, err := StaticFiles.ReadFile("static_content/generate-keystore.sh")
		Expect(err).ToNot(HaveOccurred())
		Expect(os.WriteFile(filepath.Join(root, "generate-keystore.sh"), script, 0o755)).To(Succeed())
		cmd := exec.Command("bash", filepath.Join(root, "generate-keystore.sh"))
		cmd.Env = append(os.Environ(),
			"STROOM_TLS_DIR="+tlsDir,
			"STROOM_KEYSTORE_DIR="+keystoreDir,
			"STROOM_KEYSTORE_PASSWORD=s3cret",
		)
		output, err = cmd.CombinedOutput()
		Expect(err).ToNot(HaveOccurred(), string(output))

		for _, file := range []string{"keystore.p12", "truststore.p12"} {
			path := filepath.Join(keystoreDir, file)
			info, err := os.Stat(path)
			Expect(err).ToNot(HaveOccurred())
			Expect(info.Mode().Perm()).To(Equal(os.FileMode(0o400)), file)

			// The password opens it
			check := exec.Command("openssl", "pkcs12", "-in", path, "-noout", "-passin", "pass:s3cret")
			output, err := check.CombinedOutput()
			Expect(err).ToNot(HaveOccurred(), "%v: %v", file, string(output))
		}
	})

	It("should fail when the certificate is missing", func() {
		if _, err := exec.LookPath("bash"); err != nil {
			Skip("bash is required to test the key store script")
		}
		root := GinkgoT().TempDir()
		script, err := StaticFiles.ReadFile("static_content/generate-keystore.sh")
		Expect(err).ToNot(HaveOccurred())
		Expect(os.WriteFile(filepath.Join(root, "generate-keystore.sh"), script, 0o755)).To(Succeed())
		cmd := exec.Command("bash", filepath.Join(root, "generate-keystore.sh"))
		cmd.Env = append(os.Environ(), "STROOM_TLS_DIR="+root, "STROOM_KEYSTORE_DIR="+root, "STROOM_KEYSTORE_PASSWORD=s3cret")
		Expect(cmd.Run()).ToNot(Succeed())
	})
})

var _ = Describe("Node lifecycle scripts", func() {

	var root string

	BeforeEach(func() {
		for _, tool := range []string{"bash", "jq", "base64"} {
			if _, err := exec.LookPath(tool); err != nil {
				Skip(tool + " is required to test the node lifecycle scripts")
			}
		}

		root = GinkgoT().TempDir()
		Expect(os.MkdirAll(filepath.Join(root, "scripts"), 0o755)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(root, "auth"), 0o755)).To(Succeed())
		Expect(os.MkdirAll(filepath.Join(root, "bin"), 0o755)).To(Succeed())
		for _, script := range []string{"utils.sh", "node-start.sh", "node-pre-stop.sh"} {
			content, err := StaticFiles.ReadFile("static_content/" + script)
			Expect(err).ToNot(HaveOccurred())
			Expect(os.WriteFile(filepath.Join(root, "scripts", script), content, 0o755)).To(Succeed())
		}
		Expect(os.WriteFile(filepath.Join(root, "bin", "curl"), []byte(fakeCurl), 0o755)).To(Succeed())
	})

	runScript := func(script string, managedJobs string, taskListResponses ...string) (string, error) {
		responses := strings.Join(taskListResponses, "\n") + "\n"
		Expect(os.WriteFile(filepath.Join(root, "task-list-responses"), []byte(responses), 0o644)).To(Succeed())

		cmd := exec.Command("bash", filepath.Join(root, "scripts", script))
		cmd.Env = append(os.Environ(),
			"PATH="+filepath.Join(root, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"),
			"FAKE_CURL_DIR="+root,
			"STROOM_SCRIPT_ROOT="+root,
			"STROOM_DRAIN_POLL_SECONDS=0",
			"STROOM_APP_HTTP_PORT=8080",
			"STROOM_NODE=node-1",
			"STROOM_NODE_MANAGED_JOBS="+managedJobs,
			"STROOM_OPERATOR_OPENID_CLIENT_ID=stroom-operator",
			`STROOM_OPERATOR_OPENID_CLIENT_SECRET=s3cret"value`,
		)
		output, err := cmd.CombinedOutput()
		return string(output), err
	}

	readFile := func(path ...string) string {
		content, err := os.ReadFile(filepath.Join(append([]string{root}, path...)...))
		Expect(err).ToNot(HaveOccurred())
		return string(content)
	}

	Context("node-start.sh", func() {
		It("should fetch a token, then enable the node and its jobs", func() {
			output, err := runScript("node-start.sh", "")
			Expect(err).ToNot(HaveOccurred(), output)

			requests := strings.Split(strings.TrimSpace(readFile("requests")), "\n")
			Expect(requests).To(Equal([]string{
				"http://localhost:8080/api/authproxy/v1/fetchClientCredsToken",
				"http://localhost:8080/api/node/v1/enabled/node-1",
				"http://localhost:8080/api/job/v1/setJobsEnabled/node-1",
			}))
			Expect(readFile("logs", "k8s", "node-start.log")).To(ContainSubstring("Node node-1 jobs enabled"))
		})

		It("should reuse a cached token that has not expired", func() {
			Expect(os.WriteFile(filepath.Join(root, "auth", "token"), []byte("header.eyJleHAiOjk5OTk5OTk5OTl9.signature"), 0o600)).To(Succeed())
			output, err := runScript("node-start.sh", "")
			Expect(err).ToNot(HaveOccurred(), output)

			Expect(readFile("requests")).ToNot(ContainSubstring("fetchClientCredsToken"))
			Expect(readFile("arguments")).To(ContainSubstring("Authorization:Bearer header.eyJleHAiOjk5OTk5OTk5OTl9.signature"))
		})

		It("should send the client credentials in the request body, not as an argument", func() {
			// Arguments are visible to every process in the container, in the process list
			output, err := runScript("node-start.sh", "")
			Expect(err).ToNot(HaveOccurred(), output)

			Expect(readFile("token-request-body")).To(MatchJSON(`{"clientId": "stroom-operator", "clientSecret": "s3cret\"value"}`))
			Expect(readFile("arguments")).ToNot(ContainSubstring("s3cret"))
		})

		It("should log to stderr when its log can't be written", func() {
			if os.Getuid() == 0 {
				Skip("root can write to a read-only file")
			}
			// For example, a log created by root, after the pod is changed to run as the stroom user
			Expect(os.MkdirAll(filepath.Join(root, "logs", "k8s"), 0o755)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(root, "logs", "k8s", "node-start.log"), nil, 0o444)).To(Succeed())
			output, err := runScript("node-start.sh", "")
			Expect(err).ToNot(HaveOccurred(), output)
			Expect(output).To(ContainSubstring("Node node-1 jobs enabled (unable to write to "))
		})

		It("should enable all jobs when no managed jobs are listed", func() {
			output, err := runScript("node-start.sh", "")
			Expect(err).ToNot(HaveOccurred(), output)
			Expect(readFile("arguments")).To(ContainSubstring(`{ "enabled": true, "includeJobs": [] }`))
		})

		It("should enable only the managed jobs when they are listed", func() {
			output, err := runScript("node-start.sh", "Reindex Content,Data Processor")
			Expect(err).ToNot(HaveOccurred(), output)
			Expect(readFile("arguments")).To(ContainSubstring(`{ "enabled": true, "includeJobs": ["Reindex Content","Data Processor"] }`))
		})
	})

	Context("node-pre-stop.sh", func() {
		It("should disable jobs and the node, then wait for active tasks to drain", func() {
			output, err := runScript("node-pre-stop.sh", "",
				`200 {"pageResponse":{"total":2}}`,
				`200 {"pageResponse":{"total":0}}`,
			)
			Expect(err).ToNot(HaveOccurred(), output)

			Expect(readFile("arguments")).To(ContainSubstring(`{ "enabled": false, "includeJobs": [] }`))
			Expect(readFile("requests")).To(ContainSubstring("/api/node/v1/enabled/node-1"))
			log := readFile("logs", "k8s", "pre-stop.log")
			Expect(log).To(ContainSubstring("there are still 2 active tasks"))
			Expect(log).To(ContainSubstring("All tasks drained for node node-1"))
		})

		It("should keep waiting when the active task count cannot be retrieved", func() {
			output, err := runScript("node-pre-stop.sh", "",
				`500 {"message":"Error fetching client credentials flow access token"}`,
				`200 not-json`,
				`200 {"pageResponse":{"total":0}}`,
			)
			Expect(err).ToNot(HaveOccurred(), output)

			Expect(strings.Count(readFile("requests"), "/task/v1/list/node-1")).To(Equal(3))
			log := readFile("logs", "k8s", "pre-stop.log")
			Expect(strings.Count(log, "Unable to determine the number of active tasks")).To(Equal(2))
			Expect(log).To(ContainSubstring("All tasks drained for node node-1"))
		})

		It("should stop without draining when jobs cannot be disabled", func() {
			// The token or API request fails before the drain loop is reached, e.g. during an identity provider outage
			Expect(os.WriteFile(filepath.Join(root, "bin", "curl"), []byte("#!/bin/bash\nprintf 'error\\nhttp_code=500'\n"), 0o755)).To(Succeed())
			_, err := runScript("node-pre-stop.sh", "")
			Expect(err).To(HaveOccurred())
			Expect(readFile("logs", "k8s", "api.log")).To(ContainSubstring("failed. Response code: 500"))
		})
	})
})
