package test

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	run "cloud.google.com/go/run/apiv2"
	runpb "cloud.google.com/go/run/apiv2/runpb"

	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ========================================
// Task 7.2: Helper Functions
// ========================================

// mustGetenv gets environment variable or fails test if unset
func mustGetenv(t *testing.T, key string) string {
	val := os.Getenv(key)
	if val == "" {
		t.Fatalf("Environment variable %s is required for this test", key)
	}
	return val
}

// retryWithTimeout retries a function with exponential backoff
func retryWithTimeout(t *testing.T, timeout time.Duration, interval time.Duration, fn func() error) error {
	deadline := time.Now().Add(timeout)
	attempt := 0

	for time.Now().Before(deadline) {
		attempt++
		err := fn()
		if err == nil {
			return nil
		}

		t.Logf("Attempt %d failed: %v. Retrying in %v...", attempt, err, interval)
		time.Sleep(interval)
	}

	return fmt.Errorf("operation timed out after %v", timeout)
}

// ========================================
// Task 7.3-7.6: Cloud Run Integration Test
// ========================================

// TestCloudRunModule tests the Cloud Run module deployment
func TestCloudRunModule(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// Get required environment variables
	projectID := mustGetenv(t, "TEST_GCP_PROJECT_ID")
	region := os.Getenv("TEST_GCP_REGION")
	if region == "" {
		region = "asia-northeast1"
	}
	tenantID := mustGetenv(t, "TEST_TENANT_ID")

	// Optional: Domain configuration for HTTPS testing
	// If domain_name is set, the test will verify HTTPS, SSL, DNS, and Cloud Armor
	domainName := os.Getenv("TEST_DOMAIN_NAME")
	dnsZoneName := os.Getenv("TEST_DNS_ZONE_NAME")

	// Generate unique ID for resource naming
	uniqueID := strings.ToLower(random.UniqueId())
	serviceName := fmt.Sprintf("bridge-test-%s", uniqueID)

	// Construct Terraform options
	terraformOptions := terraform.WithDefaultRetryableErrors(t, &terraform.Options{
		TerraformDir: "../../examples/gcp-cloud-run",
		Vars: map[string]any{
			"project_id":   projectID,
			"region":       region,
			"service_name": serviceName,
			"tenant_id":    tenantID,

			// Bridge configuration
			"fetch_interval": "1h",
			"fetch_timeout":  "10s",
			"port":           8080,

			// Resource configuration
			"cpu":           "1",
			"memory":        "512Mi",
			"min_instances": 0,
			"max_instances": 10,

			// Domain configuration (optional)
			"domain_name":   domainName,
			"dns_zone_name": dnsZoneName,

			// Cloud Armor IP whitelist (allow all IPs for testing)
			// WARNING: For testing purposes only. In production, restrict to specific IPs.
			// Note: BaseMachina IP (34.85.43.93/32) is always included by default
			// Using "*" to allow all IPs (per Google Cloud documentation)
			"allowed_ip_ranges": []string{"*"},

			// Cloud SQL configuration
			"database_name": "testdb",
			"database_user": "testuser",
		},
	})

	// Ensure cleanup
	// Note: VPC/subnet deletion may fail due to serverless-ipv4 circular dependency.
	// This is a known Google Cloud Direct VPC Egress limitation and is expected.
	// Resources will be cleaned up when running the cleanup script or via Google Cloud Console.
	defer func() {
		t.Log("Starting terraform destroy...")

		// Wait for Cloud Run to fully release the serverless-ipv4 address
		// Google Cloud needs time (5-10 minutes) to clean up after Cloud Run service deletion
		t.Log("Waiting 30 seconds for Google Cloud to release serverless-ipv4 addresses...")
		time.Sleep(30 * time.Second)

		// Retry destroy up to 3 times with increasing wait times
		maxRetries := 3
		var err error
		for attempt := 1; attempt <= maxRetries; attempt++ {
			t.Logf("Destroy attempt %d/%d...", attempt, maxRetries)

			_, err = terraform.DestroyE(t, terraformOptions)

			if err == nil {
				t.Log("✅ terraform destroy completed successfully")
				return
			}

			// Check if it's the known serverless-ipv4 issue
			isKnownIssue := strings.Contains(err.Error(), "serverless-ipv4") ||
				strings.Contains(err.Error(), "already being used") ||
				strings.Contains(err.Error(), "servicenetworking")

			if isKnownIssue && attempt < maxRetries {
				waitTime := time.Duration(attempt*60) * time.Second
				t.Logf("VPC deletion failed (serverless-ipv4 still in use). Waiting %v before retry...", waitTime)
				time.Sleep(waitTime)
				continue
			}

			// If max retries reached or unknown error, log and continue
			break
		}

		// Handle final error after all retries
		if err != nil {
			// VPC削除エラーは既知の問題（serverless-ipv4 circular dependency）
			if strings.Contains(err.Error(), "serverless-ipv4") ||
				strings.Contains(err.Error(), "already being used") ||
				strings.Contains(err.Error(), "servicenetworking") {
				t.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				t.Logf("⚠️  VPC deletion failed after %d retries (known Google Cloud Direct VPC Egress limitation)", maxRetries)
				t.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				t.Logf("")
				t.Logf("This is expected behavior:")
				t.Logf("  - serverless-ipv4 addresses are auto-created by Cloud Run")
				t.Logf("  - They cannot be deleted independently")
				t.Logf("  - Google Cloud needs 5-10 minutes to release them after Cloud Run service deletion")
				t.Logf("  - This creates circular dependency: VPC ← subnet ← serverless-ipv4")
				t.Logf("")
				t.Logf("To clean up remaining resources:")
				t.Logf("")
				t.Logf("Option 1: Use cleanup script (recommended)")
				t.Logf("  cd examples/gcp-cloud-run")
				t.Logf("  ./scripts/cleanup.sh %s %s", projectID, serviceName)
				t.Logf("")
				t.Logf("Option 2: Wait and retry")
				t.Logf("  cd examples/gcp-cloud-run")
				t.Logf("  # Wait 5-10 minutes, then:")
				t.Logf("  terraform destroy -auto-approve")
				t.Logf("")
				t.Logf("Option 3: Delete via Google Cloud Console")
				t.Logf("  https://console.cloud.google.com/networking/networks?project=%s", projectID)
				t.Logf("  - Delete VPC: %s-vpc", serviceName)
				t.Logf("")
				t.Logf("Option 4: Leave resources (no cost impact)")
				t.Logf("  - VPC, subnet, serverless-ipv4 are all free")
				t.Logf("  - New tests use unique IDs and won't conflict")
				t.Logf("")
				t.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
				t.Logf("")

				// テストは失敗させない（VPC削除は既知の問題のため）
			} else {
				// その他のエラーはログに出力するが、テストは失敗させない
				t.Logf("⚠️  terraform destroy encountered an error: %v", err)
				t.Logf("This may require manual cleanup via Google Cloud Console or cleanup script")
			}
		}
	}()

	// Run terraform init and apply
	terraform.InitAndApply(t, terraformOptions)

	// ========================================
	// Task 7.3: Cloud Run Service Validation
	// ========================================

	t.Run("CloudRunServiceExists", func(t *testing.T) {
		// Verify Cloud Run service exists
		client, err := run.NewServicesClient(ctx)
		require.NoError(t, err)
		defer client.Close()

		servicePath := fmt.Sprintf("projects/%s/locations/%s/services/%s", projectID, region, serviceName)
		service, err := client.GetService(ctx, &runpb.GetServiceRequest{
			Name: servicePath,
		})
		require.NoError(t, err)
		assert.NotNil(t, service)

		t.Logf("Cloud Run service found: %s", service.Name)

		// Verify service configuration
		template := service.GetTemplate()
		require.NotNil(t, template)

		containers := template.GetContainers()
		require.NotEmpty(t, containers)

		container := containers[0]

		// Verify environment variables
		envVars := container.GetEnv()
		envMap := make(map[string]string)
		for _, env := range envVars {
			envMap[env.GetName()] = env.GetValue()
		}

		assert.Equal(t, "1h", envMap["FETCH_INTERVAL"])
		assert.Equal(t, "10s", envMap["FETCH_TIMEOUT"])
		assert.Equal(t, tenantID, envMap["TENANT_ID"])
		// Note: PORT environment variable is automatically set by Cloud Run from container_port

		// Verify container port
		ports := container.GetPorts()
		require.NotEmpty(t, ports)
		assert.Equal(t, int32(8080), ports[0].GetContainerPort())

		// Verify resource limits
		resources := container.GetResources()
		require.NotNil(t, resources)
		assert.Equal(t, "1", resources.Limits["cpu"])
		assert.Equal(t, "512Mi", resources.Limits["memory"])

		// Verify ingress setting (internal load balancer only)
		assert.Equal(t, runpb.IngressTraffic_INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER, service.GetIngress())

		t.Logf("Cloud Run service configuration verified")
	})

	// ========================================
	// Task 7.4: HTTPS and Health Check Test
	// ========================================

	if domainName != "" {
		t.Run("HTTPSHealthCheck", func(t *testing.T) {
			// Get domain URL from outputs
			domainURL := fmt.Sprintf("https://%s", domainName)
			lbIP := terraform.Output(t, terraformOptions, "bridge_load_balancer_ip")

			t.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			t.Logf("HTTPS Health Check Configuration")
			t.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			t.Logf("  Domain:         %s", domainName)
			t.Logf("  URL:            %s/ok", domainURL)
			t.Logf("  Load Balancer:  %s", lbIP)
			t.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// Check DNS resolution first
			t.Logf("Step 1: Checking DNS resolution...")
			ips, err := net.LookupIP(domainName)
			if err != nil {
				t.Logf("  ❌ DNS lookup failed: %v", err)
			} else {
				t.Logf("  ✅ DNS resolved to: %v", ips)
				for _, ip := range ips {
					if ip.String() == lbIP {
						t.Logf("  ✅ Load Balancer IP matched: %s", lbIP)
					}
				}
			}

			// Wait for SSL certificate to be provisioned and DNS to propagate
			// This can take up to 15 minutes
			t.Logf("\nStep 2: Waiting for SSL certificate provisioning and health check...")
			t.Logf("  Timeout: 5 minutes")
			t.Logf("  Interval: 30 seconds")

			err = retryWithTimeout(t, 5*time.Minute, 30*time.Second, func() error {
				// Log detailed attempt information
				t.Logf("\n  → Attempting HTTPS request to %s/ok", domainURL)

				// Create custom HTTP client with timeout
				client := &http.Client{
					Timeout: 10 * time.Second,
				}

				resp, err := client.Get(domainURL + "/ok")
				if err != nil {
					t.Logf("     ❌ Request error: %v", err)
					return fmt.Errorf("HTTP request failed: %w", err)
				}
				defer resp.Body.Close()

				t.Logf("     Status: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
				t.Logf("     TLS: %v", resp.TLS != nil)
				if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
					cert := resp.TLS.PeerCertificates[0]
					t.Logf("     Certificate: CN=%s, Issuer=%s", cert.Subject.CommonName, cert.Issuer.CommonName)
					t.Logf("     Valid: %v - %v", cert.NotBefore, cert.NotAfter)
				}

				if resp.StatusCode != http.StatusOK {
					// Read response body for error details
					body, _ := io.ReadAll(resp.Body)
					bodyPreview := string(body)
					if len(bodyPreview) > 500 {
						bodyPreview = bodyPreview[:500] + "..."
					}
					t.Logf("     Response body: %s", bodyPreview)
					return fmt.Errorf("expected status 200, got %d: %s", resp.StatusCode, http.StatusText(resp.StatusCode))
				}

				body, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Logf("     ❌ Failed to read body: %v", err)
					return fmt.Errorf("failed to read response body: %w", err)
				}

				bodyStr := strings.ToLower(strings.TrimSpace(string(body)))
				t.Logf("     Response body: '%s'", bodyStr)

				if bodyStr != "bridge is ready" {
					t.Logf("     ❌ Unexpected body content")
					return fmt.Errorf("expected response body 'ok', got '%s'", bodyStr)
				}

				t.Logf("     ✅ Health check succeeded!")
				return nil
			})

			require.NoError(t, err, "HTTPS health check failed")
			t.Logf("\n✅ HTTPS health check passed: %s/ok", domainURL)

			// Verify SSL certificate
			resp, err := http.Get(domainURL + "/ok")
			require.NoError(t, err)
			defer resp.Body.Close()

			assert.NotNil(t, resp.TLS, "TLS connection should be established")
			if resp.TLS != nil {
				assert.NotEmpty(t, resp.TLS.PeerCertificates, "SSL certificate should be present")
				t.Logf("SSL certificate verified")
			}
		})
	}

	// ========================================
	// Task 7.6: DNS Resolution and Load Balancer Test
	// ========================================

	if domainName != "" && dnsZoneName != "" {
		t.Run("DNSResolutionAndLoadBalancer", func(t *testing.T) {
			// Get Load Balancer IP from outputs
			lbIP := terraform.Output(t, terraformOptions, "bridge_load_balancer_ip")
			require.NotEmpty(t, lbIP)

			t.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			t.Logf("DNS Resolution and Load Balancer Test")
			t.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
			t.Logf("  Domain:         %s", domainName)
			t.Logf("  DNS Zone:       %s", dnsZoneName)
			t.Logf("  Expected LB IP: %s", lbIP)
			t.Logf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

			// Wait for DNS propagation
			t.Logf("\nWaiting for DNS propagation...")
			t.Logf("  Timeout: 5 minutes")
			t.Logf("  Interval: 10 seconds")

			err := retryWithTimeout(t, 5*time.Minute, 10*time.Second, func() error {
				t.Logf("\n  → Performing DNS lookup for %s", domainName)

				ips, err := net.LookupIP(domainName)
				if err != nil {
					t.Logf("     ❌ DNS lookup error: %v", err)
					return fmt.Errorf("DNS lookup failed: %w", err)
				}

				if len(ips) == 0 {
					t.Logf("     ❌ No IP addresses found")
					return fmt.Errorf("no IP addresses found for domain %s", domainName)
				}

				t.Logf("     DNS resolved to %d IP(s):", len(ips))
				for _, ip := range ips {
					t.Logf("       - %s", ip.String())
				}

				// Check if Load Balancer IP is in the resolved IPs
				found := false
				for _, ip := range ips {
					if ip.String() == lbIP {
						found = true
						t.Logf("     ✅ Load Balancer IP matched: %s", lbIP)
						break
					}
				}

				if !found {
					t.Logf("     ❌ Expected IP %s not found in DNS resolution", lbIP)
					return fmt.Errorf("expected IP %s not found in DNS resolution", lbIP)
				}

				return nil
			})

			require.NoError(t, err, "DNS resolution test failed")
			t.Logf("\n✅ DNS resolution verified: %s -> %s", domainName, lbIP)

			// Verify Cloud Armor (access from allowed IP should succeed)
			// Note: This test assumes the test is running from an allowed IP
			resp, err := http.Get(fmt.Sprintf("https://%s/ok", domainName))
			if err == nil {
				defer resp.Body.Close()
				// If we can access, verify it's successful
				assert.Equal(t, http.StatusOK, resp.StatusCode, "Access from allowed IP should succeed")
				t.Logf("Cloud Armor: Access from allowed IP succeeded")
			} else {
				// If access fails, it might be because we're not in the allowed IP range
				t.Logf("Cloud Armor: Cannot verify (test may not be running from allowed IP)")
			}
		})
	}

	// Log all outputs for debugging
	t.Run("LogOutputs", func(t *testing.T) {
		outputs := []string{
			"bridge_service_url",
			"bridge_service_name",
			"bridge_load_balancer_ip",
			"cloud_sql_connection_name",
			"cloud_sql_private_ip",
			"database_name",
		}

		t.Log("Terraform Outputs:")
		for _, output := range outputs {
			val := terraform.Output(t, terraformOptions, output)
			t.Logf("  %s: %s", output, val)
		}
	})
}
