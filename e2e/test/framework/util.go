package framework

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"strings"
	"time"

	"github.com/appscode/go/wait"
	"github.com/golang/glog"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	scriptDirectory = "scripts"
	RetryInterval   = 5 * time.Second
	RetryTimeout    = 15 * time.Minute
	caCert          = `-----BEGIN CERTIFICATE-----
MIIFejCCA2KgAwIBAgIJAN7D2Ju254yUMA0GCSqGSIb3DQEBCwUAMFIxCzAJBgNV
BAYTAkFVMRMwEQYDVQQIDApTb21lLVN0YXRlMSEwHwYDVQQKDBhJbnRlcm5ldCBX
aWRnaXRzIFB0eSBMdGQxCzAJBgNVBAMMAmNhMB4XDTE5MDQwOTA5MzYxNFoXDTI5
MDQwNjA5MzYxNFowUjELMAkGA1UEBhMCQVUxEzARBgNVBAgMClNvbWUtU3RhdGUx
ITAfBgNVBAoMGEludGVybmV0IFdpZGdpdHMgUHR5IEx0ZDELMAkGA1UEAwwCY2Ew
ggIiMA0GCSqGSIb3DQEBAQUAA4ICDwAwggIKAoICAQDoTwE1kijjrhCcGXSPyHlf
7NngxPCFuFqVdRvG4DrrdL7YW3iEovAXTbuoyiPpF/U9T5BfDVs2dCEHGlpiOADR
tA/Z5mFbVcefOCBL+rL2sTN2o19U7eimcZjH1xN1L5j2RkYmRAoI+nwG/g5NehOu
YM930oPqe3vOYevOHBCebHuKc7zaM31AtKcDG0IjIJ1ZdJy91+rx8Prb+IxTIKZl
Ca/e0e6iZWCPp5kaJyNUGZkjjcRVzFM79xVf34DEuS+N1RZP7EevM0bfHehJfSpU
M6gfsrL9WctD0nGJd2YsH9hLCub2G7emgiV7dvN1R0QW9ijguwZ9aBemiat5AnGs
QHSR+WRijZNjHTWY4DEaTNWecDd2Tz37RNN9Ow8FThERwZVnpji1kcijEg4g7Ppy
9P6tdavjkFVW0xOieInjS/m5Bxj2a44UT1JshNr1M4HGXvqUcCFS4vhytIc05lOv
X20NR+C+RgNy7G14Hz/3+qRo9hlkonyTJAoU++2vgsaNmmhcU6fGgYpARHm1Y675
pGrgZAcjFcsG84q0dSdr6AeY+6+1UyS6pktBobXIiciSPmseHJ24dRd06OYQMxQ3
ccOZhZ3cNy8OMT9eUwcjnif36BVmZdCObJexqXq/cSVX3IhhaQhLLfN9ZyGDkxWl
N5ehRMCabgv3mQCDd/9HMwIDAQABo1MwUTAdBgNVHQ4EFgQUC2AMOf90/zpuQ588
rPLfe7EukIUwHwYDVR0jBBgwFoAUC2AMOf90/zpuQ588rPLfe7EukIUwDwYDVR0T
AQH/BAUwAwEB/zANBgkqhkiG9w0BAQsFAAOCAgEAHopjHkeIciVtlAyAPEfh/pnf
r91H1aQMPmHisqlveM3Bz9MOIa9a26YO+ZzCPozALxkJIjdp7L3L8Q8CuLmkC4YV
6nHvSLaC/82UGoiRGyjdFh30puqekWMZ62ZrQLpCr0DzOJrarslLM0fONqpjDTWP
8OXyRcnVSbFB1n5XUoviMTTxYOQ3HQe8b3Tt7GO/9w6dWkkSX1Vy4RmzNt7fb9K5
mxu/n+SVu+2iQX9oEWq2rpvsD3RGnhewCPlZU8NQYKb72K00kEcG/J+WU1IPtkq0
JaU5TDMMzfp3PMYxCzYD9pdM8J0N0zJac2t9hkx7H83jy/TfLrmDvB6nCK8N3+6j
8In6RwYw4XJ41AWsJpGXBpvYCq5GJjdogEi9IaBXSmtVPYm0NURYbephk+Wg0oyk
ESk4cyWUhYG8mcMyORc8lzOQ79YT6A5QnitTGCVQGTlnNRjevtfhAFEXr9e8UZFq
oWtfEdltH6ElGDpivwuOERAN9v3GoPlifpo1UDElnPJft+C0cRv0YpPwvwJTy1MU
q1op/4Z/7SHzFWTSyRZqvI41AsLImylzfZ0w9U8sogd4pHv30kGc9+LhqrsfLDvK
9XedVoWJx/x3i8BUhVDyd4FyVWHCf9N/6a9HzbFWT8QZTBk5pErTaFiTi5TQxoi7
ER4ILjvRX7mLWUGhN58=
-----END CERTIFICATE-----`
	Domain = "linode.test"
)

func RunScript(script string, args ...string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	return runCommand(path.Join(wd, scriptDirectory, script), args...)
}

func runCommand(cmd string, args ...string) error {
	c := exec.Command(cmd, args...)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	c.Env = append(c.Env, append(os.Environ())...)
	glog.Infof("Running command %q\n", cmd)
	return c.Run()
}

func deleteInForeground() *metav1.DeleteOptions {
	policy := metav1.DeletePropagationForeground
	graceSeconds := int64(0)
	return &metav1.DeleteOptions{
		PropagationPolicy:  &policy,
		GracePeriodSeconds: &graceSeconds,
	}
}

func getHTTPSResponse(domain, ip, port string) (string, error) {
	rootCAs, _ := x509.SystemCertPool()
	if rootCAs == nil {
		rootCAs = x509.NewCertPool()
	}

	if ok := rootCAs.AppendCertsFromPEM([]byte(caCert)); !ok {
		log.Println("No certs appended, using system certs only")
	}

	config := &tls.Config{
		RootCAs: rootCAs,
	}

	dialer := &net.Dialer{
		Timeout:   30 * time.Second,
		KeepAlive: 30 * time.Second,
		DualStack: true,
	}
	dialContext := func(ctx context.Context, network, addr string) (net.Conn, error) {
		if addr == domain+":"+port {
			addr = ip + ":" + port
		}
		return dialer.DialContext(ctx, network, addr)
	}

	tr := &http.Transport{
		TLSClientConfig: config,
		DialContext:     dialContext,
	}
	client := &http.Client{Transport: tr}

	log.Println("Waiting for response from https://" + ip + ":" + port)
	u := "https://" + domain + ":" + port
	req, err := http.NewRequest(http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	bodyString := string(bodyBytes)

	return bodyString, nil
}

func WaitForHTTPSResponse(link string, podName string) error {
	return wait.PollImmediate(RetryInterval, RetryTimeout, func() (bool, error) {
		u, err := url.Parse(link)
		if err != nil {
			return false, nil
		}
		ipPort := strings.Split(u.Host, ":")

		resp, err := getHTTPSResponse(Domain, ipPort[0], ipPort[1])
		if err != nil {
			return false, nil
		}

		if strings.Contains(resp, podName) {
			log.Println("Got response from " + podName + " using url " + link)
			return true, nil
		}

		return false, nil
	})
}

func getHTTPResponse(link string) (bool, string, error) {
	resp, err := http.Get(link)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()

	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return false, "", err
	}

	return resp.StatusCode == 200, string(bodyBytes), nil
}

func WaitForHTTPResponse(link string, podName string) error {
	return wait.PollImmediate(RetryInterval, RetryTimeout, func() (bool, error) {
		ok, _, err := getHTTPResponse(link)
		if err != nil {
			return false, nil
		}
		if ok {
			log.Println("Got response from " + podName + " using url " + link)
			return true, nil
		}

		return false, nil
	})
}
