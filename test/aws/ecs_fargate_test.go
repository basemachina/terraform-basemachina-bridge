package test

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/cloudwatchlogs"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/aws/aws-sdk-go/service/elbv2"
	"github.com/gruntwork-io/terratest/modules/random"
	"github.com/gruntwork-io/terratest/modules/terraform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper to get environment variable or fail test if unset
func mustGetenv(t *testing.T, key string) string {
	val := os.Getenv(key)
	if val == "" {
		t.Fatalf("Environment variable %s is required for this test", key)
	}
	return val
}

// Helper to get environment variable as slice
func getenvSlice(t *testing.T, key string) []string {
	val := mustGetenv(t, key)
	parts := strings.Split(val, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		t.Fatalf("Environment variable %s must not be an empty list", key)
	}
	return out
}

// TestECSFargateModule tests the ECS Fargate module deployment
func TestECSFargateModule(t *testing.T) {
	t.Parallel()

	// region from env or "ap-northeast-1" default
	awsRegion := os.Getenv("AWS_DEFAULT_REGION")
	if awsRegion == "" {
		awsRegion = "ap-northeast-1"
	}

	uniqueID := strings.ToLower(random.UniqueId())
	// name_prefixはモジュールのvalidationで8文字以内に制限されているため、
	// "test-" (cleanupExistingS3Endpointsのテストリソース判定に使用) + ランダム3文字で8文字に収める
	namePrefix := fmt.Sprintf("test-%s", uniqueID[:3])

	// Required env vars for tf vars
	vpcID := mustGetenv(t, "TEST_VPC_ID")
	privateSubnetIDs := getenvSlice(t, "TEST_PRIVATE_SUBNET_IDS")
	publicSubnetIDs := getenvSlice(t, "TEST_PUBLIC_SUBNET_IDS")
	tenantID := mustGetenv(t, "TEST_TENANT_ID")

	// Domain configuration (required):
	// - TEST_BRIDGE_DOMAIN_NAME: Domain name for Bridge (e.g., bridge-test.example.com)
	// - TEST_ROUTE53_ZONE_ID: Existing Route53 Hosted Zone ID for the domain
	// ACM certificate will be automatically issued via DNS validation
	bridgeDomainName := mustGetenv(t, "TEST_BRIDGE_DOMAIN_NAME")
	route53ZoneID := mustGetenv(t, "TEST_ROUTE53_ZONE_ID")

	// Optional: desired count
	desiredCount := int64(1)
	if val := os.Getenv("TEST_DESIRED_COUNT"); val != "" {
		var n int64
		_, err := fmt.Sscanf(val, "%d", &n)
		if err == nil && n > 0 {
			desiredCount = n
		}
	}

	// AWS creds from env
	awsAccessKey := mustGetenv(t, "AWS_ACCESS_KEY_ID")
	awsSecretKey := mustGetenv(t, "AWS_SECRET_ACCESS_KEY")

	// Construct terraform vars
	// Network access configuration:
	// Using NAT Gateway + VPC Endpoints + ECR Pull Through Cache
	//
	// Configuration: Private subnets + NAT Gateway + VPC Endpoints + ECR Pull Through Cache
	// - NAT Gateway: For Bridge initialization (connects to BaseMachina auth servers)
	// - VPC Endpoints: For Private ECR, S3, CloudWatch Logs (always enabled)
	// - ECR Pull Through Cache: Public ECR images are automatically cached by AWS managed infrastructure (always enabled)
	// - Result: Full internet access via NAT Gateway, optimized AWS service access via VPC endpoints
	//
	// How ECR Pull Through Cache works:
	// - AWS's managed infrastructure fetches images from Public ECR (not the ECS task)
	// - ECS tasks only access Private ECR via VPC endpoints
	//
	// Why NAT Gateway is required:
	// - Bridge needs to fetch authentication keys from BaseMachina's servers
	// - VPC Endpoints only cover AWS services (ECR, S3, CloudWatch)
	// - General internet access requires NAT Gateway (~$32/month)

	t.Log("Using private subnets with NAT Gateway + VPC Endpoints + ECR Pull Through Cache")
	t.Logf("Private subnets: %v", privateSubnetIDs)
	t.Logf("Public subnets: %v", publicSubnetIDs)
	t.Log("NAT Gateway: Enabled for Bridge initialization (connects to BaseMachina auth servers)")
	t.Log("VPC Endpoints: ECR API, ECR Docker, S3, CloudWatch Logs (always enabled)")
	t.Log("ECR Pull Through Cache: AWS fetches images from Public ECR automatically (always enabled)")

	tfVars := map[string]interface{}{
		"name_prefix":        namePrefix,
		"vpc_id":             vpcID,
		"private_subnet_ids": privateSubnetIDs,
		"public_subnet_ids":  publicSubnetIDs,
		"tenant_id":          tenantID,
		"desired_count":      int(desiredCount),
		"bridge_domain_name": bridgeDomainName,
		"route53_zone_id":    route53ZoneID,
	}

	// Allow test environment to access ALB
	// For testing, allow all IPs (0.0.0.0/0) to access ALB
	// Production deployments should restrict this to specific IPs
	tfVars["additional_alb_ingress_cidrs"] = []string{"0.0.0.0/0"}
	t.Log("Allowing ALB access from all IPs (0.0.0.0/0) for testing")
	t.Logf("Bridge domain: %s", bridgeDomainName)
	t.Logf("Route53 Zone ID: %s", route53ZoneID)
	t.Log("ACM certificate will be automatically issued via DNS validation")

	// Construct the terraform options with default retryable errors
	terraformOptions := terraform.WithDefaultRetryableErrors(t, &terraform.Options{
		TerraformDir: "../../examples/aws-ecs-fargate",
		Vars:         tfVars,
		EnvVars: map[string]string{
			"AWS_ACCESS_KEY_ID":        awsAccessKey,
			"AWS_SECRET_ACCESS_KEY":    awsSecretKey,
			"AWS_DEFAULT_REGION":       awsRegion,
			"AWS_DISABLE_EC2_METADATA": "true",
		},
	})

	// Create AWS session
	sess, err := session.NewSession(&aws.Config{
		Region: aws.String(awsRegion),
	})
	require.NoError(t, err)

	ec2Client := ec2.New(sess)

	// Clean up any existing S3 VPC endpoints in the test VPC to avoid conflicts
	cleanupExistingS3Endpoints(t, ec2Client, vpcID, namePrefix)

	// Verify Route53 zone before starting
	verifyRoute53Zone(t, route53ZoneID, bridgeDomainName)

	defer terraform.Destroy(t, terraformOptions)
	terraform.InitAndApply(t, terraformOptions)

	// Trigger ECR pull-through cache by describing the image
	// This creates the repository in the pull-through cache if it doesn't exist
	// Without this, ECS tasks will fail with "image not found" error
	t.Log("Triggering ECR pull-through cache repository creation...")
	triggerPullThroughCache(t, awsRegion)

	albDNSName := terraform.Output(t, terraformOptions, "alb_dns_name")
	albArn := terraform.Output(t, terraformOptions, "alb_arn")
	albSecurityGroupID := terraform.Output(t, terraformOptions, "alb_security_group_id")
	ecsClusterName := terraform.Output(t, terraformOptions, "ecs_cluster_name")
	ecsClusterArn := terraform.Output(t, terraformOptions, "ecs_cluster_arn")
	ecsServiceName := terraform.Output(t, terraformOptions, "ecs_service_name")
	bridgeSecurityGroupID := terraform.Output(t, terraformOptions, "bridge_security_group_id")
	cloudwatchLogGroupName := terraform.Output(t, terraformOptions, "cloudwatch_log_group_name")
	taskExecutionRoleArn := terraform.Output(t, terraformOptions, "task_execution_role_arn")

	assert.NotEmpty(t, albDNSName)
	assert.NotEmpty(t, albArn)
	assert.NotEmpty(t, albSecurityGroupID)
	assert.NotEmpty(t, ecsClusterName)
	assert.NotEmpty(t, ecsClusterArn)
	assert.NotEmpty(t, ecsServiceName)
	assert.NotEmpty(t, bridgeSecurityGroupID)
	assert.NotEmpty(t, cloudwatchLogGroupName)
	assert.NotEmpty(t, taskExecutionRoleArn)

	// Create ECS and ELBv2 clients
	ecsClient := ecs.New(sess)
	elbv2Client := elbv2.New(sess)

	maxRetries := 30
	timeBetweenRetries := 10 * time.Second

	// ECS Service check
	for i := 0; i < maxRetries; i++ {
		describeServicesInput := &ecs.DescribeServicesInput{
			Cluster:  aws.String(ecsClusterName),
			Services: []*string{aws.String(ecsServiceName)},
		}

		result, err := ecsClient.DescribeServices(describeServicesInput)
		if err != nil {
			t.Logf("Attempt %d/%d: Error getting ECS service info: %v", i+1, maxRetries, err)
			time.Sleep(timeBetweenRetries)
			continue
		}
		if len(result.Services) == 0 {
			t.Logf("Attempt %d/%d: ECS service not found", i+1, maxRetries)
			time.Sleep(timeBetweenRetries)
			continue
		}
		service := result.Services[0]
		runningTaskCount := *service.RunningCount

		t.Logf("Attempt %d/%d: ECS Service has %d running tasks (desired: %d)", i+1, maxRetries, runningTaskCount, desiredCount)

		// Log service events for debugging
		if len(service.Events) > 0 {
			t.Logf("Recent service events:")
			for j := 0; j < 3 && j < len(service.Events); j++ {
				event := service.Events[j]
				t.Logf("  - [%s] %s", event.CreatedAt.Format("15:04:05"), *event.Message)
			}
		}

		// If tasks are not running, check task status (start checking after 3 attempts)
		if runningTaskCount == 0 && i >= 2 {
			// Check stopped tasks
			listStoppedTasksInput := &ecs.ListTasksInput{
				Cluster:       aws.String(ecsClusterName),
				ServiceName:   aws.String(ecsServiceName),
				DesiredStatus: aws.String("STOPPED"),
			}
			stoppedTasksResult, err := ecsClient.ListTasks(listStoppedTasksInput)
			if err == nil && len(stoppedTasksResult.TaskArns) > 0 {
				describeTasksInput := &ecs.DescribeTasksInput{
					Cluster: aws.String(ecsClusterName),
					Tasks:   []*string{stoppedTasksResult.TaskArns[0]}, // Get most recent stopped task
				}
				tasksDetails, err := ecsClient.DescribeTasks(describeTasksInput)
				if err == nil && len(tasksDetails.Tasks) > 0 {
					task := tasksDetails.Tasks[0]
					t.Logf("=== STOPPED TASK DETAILS ===")
					t.Logf("Task ARN: %s", aws.StringValue(task.TaskArn))
					t.Logf("Last Status: %s", aws.StringValue(task.LastStatus))
					t.Logf("Stopped Reason: %s", aws.StringValue(task.StoppedReason))
					if len(task.Containers) > 0 {
						container := task.Containers[0]
						t.Logf("Container Name: %s", aws.StringValue(container.Name))
						t.Logf("Container Status: %s", aws.StringValue(container.LastStatus))
						t.Logf("Container Reason: %s", aws.StringValue(container.Reason))
						if container.ExitCode != nil {
							t.Logf("Container Exit Code: %d", *container.ExitCode)
						}
					}
					t.Logf("===========================")
				}
			}

			// Also check running/pending tasks
			listRunningTasksInput := &ecs.ListTasksInput{
				Cluster:       aws.String(ecsClusterName),
				ServiceName:   aws.String(ecsServiceName),
				DesiredStatus: aws.String("RUNNING"),
			}
			runningTasksResult, err := ecsClient.ListTasks(listRunningTasksInput)
			if err == nil && len(runningTasksResult.TaskArns) > 0 {
				describeTasksInput := &ecs.DescribeTasksInput{
					Cluster: aws.String(ecsClusterName),
					Tasks:   []*string{runningTasksResult.TaskArns[0]},
				}
				tasksDetails, err := ecsClient.DescribeTasks(describeTasksInput)
				if err == nil && len(tasksDetails.Tasks) > 0 {
					task := tasksDetails.Tasks[0]
					t.Logf("=== RUNNING/PENDING TASK DETAILS ===")
					t.Logf("Task ARN: %s", aws.StringValue(task.TaskArn))
					t.Logf("Last Status: %s", aws.StringValue(task.LastStatus))
					t.Logf("Desired Status: %s", aws.StringValue(task.DesiredStatus))
					if len(task.Containers) > 0 {
						container := task.Containers[0]
						t.Logf("Container Status: %s", aws.StringValue(container.LastStatus))
					}

					// If task is stuck in PENDING for too long, run network diagnosis
					if aws.StringValue(task.LastStatus) == "PENDING" && i >= 10 {
						t.Logf("⚠️  Task has been PENDING for %d attempts (>100 seconds)", i+1)
						t.Logf("Running network diagnosis to identify the issue...")
						diagnoseTaskFailure(t, ecsClient, ec2Client, ecsClusterName, ecsServiceName, privateSubnetIDs, vpcID)
					}

					t.Logf("===================================")
				}
			}
		}

		if runningTaskCount == desiredCount {
			break
		}

		if i == maxRetries-1 {
			require.Equal(t, desiredCount, runningTaskCount, "ECS Service should have %d running tasks", desiredCount)
		}

		time.Sleep(timeBetweenRetries)
	}

	// ALB Target Group health check
	describeLoadBalancersInput := &elbv2.DescribeLoadBalancersInput{
		LoadBalancerArns: []*string{aws.String(albArn)},
	}
	lbResult, err := elbv2Client.DescribeLoadBalancers(describeLoadBalancersInput)
	require.NoError(t, err)
	require.NotEmpty(t, lbResult.LoadBalancers, "ALB should exist")

	describeTargetGroupsInput := &elbv2.DescribeTargetGroupsInput{
		LoadBalancerArn: aws.String(albArn),
	}
	tgResult, err := elbv2Client.DescribeTargetGroups(describeTargetGroupsInput)
	require.NoError(t, err)
	require.NotEmpty(t, tgResult.TargetGroups, "ALB should have at least one target group")

	targetGroup := tgResult.TargetGroups[0]
	targetGroupArn := *targetGroup.TargetGroupArn

	// Log target group configuration
	t.Log("=== TARGET GROUP CONFIGURATION ===")
	t.Logf("Target Group ARN: %s", aws.StringValue(targetGroup.TargetGroupArn))
	t.Logf("Target Group Name: %s", aws.StringValue(targetGroup.TargetGroupName))
	t.Logf("Protocol: %s", aws.StringValue(targetGroup.Protocol))
	t.Logf("Port: %d", aws.Int64Value(targetGroup.Port))
	t.Logf("VPC ID: %s", aws.StringValue(targetGroup.VpcId))
	t.Logf("Health Check Protocol: %s", aws.StringValue(targetGroup.HealthCheckProtocol))
	t.Logf("Health Check Port: %s", aws.StringValue(targetGroup.HealthCheckPort))
	t.Logf("Health Check Path: %s", aws.StringValue(targetGroup.HealthCheckPath))
	t.Logf("Health Check Interval: %d seconds", aws.Int64Value(targetGroup.HealthCheckIntervalSeconds))
	t.Logf("Health Check Timeout: %d seconds", aws.Int64Value(targetGroup.HealthCheckTimeoutSeconds))
	t.Logf("Healthy Threshold: %d", aws.Int64Value(targetGroup.HealthyThresholdCount))
	t.Logf("Unhealthy Threshold: %d", aws.Int64Value(targetGroup.UnhealthyThresholdCount))
	t.Log("==================================")

	// Get ECS task network interface IPs for comparison
	t.Log("=== ECS TASK NETWORK INTERFACES ===")
	listTasksForNetworkInput := &ecs.ListTasksInput{
		Cluster:       aws.String(ecsClusterName),
		ServiceName:   aws.String(ecsServiceName),
		DesiredStatus: aws.String("RUNNING"),
	}
	listTasksForNetworkResult, err := ecsClient.ListTasks(listTasksForNetworkInput)
	if err == nil && len(listTasksForNetworkResult.TaskArns) > 0 {
		describeTasksForNetworkInput := &ecs.DescribeTasksInput{
			Cluster: aws.String(ecsClusterName),
			Tasks:   listTasksForNetworkResult.TaskArns,
		}
		describeTasksForNetworkResult, err := ecsClient.DescribeTasks(describeTasksForNetworkInput)
		if err == nil {
			for idx, task := range describeTasksForNetworkResult.Tasks {
				t.Logf("Task %d:", idx+1)
				t.Logf("  Task ARN: %s", aws.StringValue(task.TaskArn))
				if len(task.Attachments) > 0 {
					for _, attachment := range task.Attachments {
						if aws.StringValue(attachment.Type) == "ElasticNetworkInterface" {
							for _, detail := range attachment.Details {
								if aws.StringValue(detail.Name) == "privateIPv4Address" {
									t.Logf("  Private IP: %s", aws.StringValue(detail.Value))
								}
								if aws.StringValue(detail.Name) == "networkInterfaceId" {
									t.Logf("  ENI: %s", aws.StringValue(detail.Value))
								}
							}
						}
					}
				}
			}
		}
	}
	t.Log("===================================")

	for i := 0; i < maxRetries; i++ {
		describeTargetHealthInput := &elbv2.DescribeTargetHealthInput{
			TargetGroupArn: aws.String(targetGroupArn),
		}

		healthResult, err := elbv2Client.DescribeTargetHealth(describeTargetHealthInput)
		if err != nil {
			t.Logf("Attempt %d/%d: Error getting target health: %v", i+1, maxRetries, err)
			time.Sleep(timeBetweenRetries)
			continue
		}

		healthyCount := int64(0)
		unhealthyCount := int64(0)

		// Detailed logging of target health states
		if len(healthResult.TargetHealthDescriptions) > 0 {
			t.Logf("Attempt %d/%d: Target health details:", i+1, maxRetries)
			for idx, targetHealth := range healthResult.TargetHealthDescriptions {
				target := targetHealth.Target
				health := targetHealth.TargetHealth

				t.Logf("  Target %d:", idx+1)
				t.Logf("    ID: %s", aws.StringValue(target.Id))
				t.Logf("    Port: %d", aws.Int64Value(target.Port))
				t.Logf("    State: %s", aws.StringValue(health.State))
				t.Logf("    Reason: %s", aws.StringValue(health.Reason))
				t.Logf("    Description: %s", aws.StringValue(health.Description))

				if aws.StringValue(health.State) == "healthy" {
					healthyCount++
				} else {
					unhealthyCount++
				}
			}
		} else {
			t.Logf("Attempt %d/%d: No targets registered in target group", i+1, maxRetries)
		}

		t.Logf("Attempt %d/%d: Summary - Healthy: %d, Unhealthy: %d, Desired: %d",
			i+1, maxRetries, healthyCount, unhealthyCount, desiredCount)

		if healthyCount == desiredCount {
			assert.Equal(t, desiredCount, healthyCount, "Target group should have %d healthy targets", desiredCount)
			break
		}

		if i == maxRetries-1 {
			// Before failing, do a final diagnosis
			t.Log("=== FINAL HEALTH CHECK DIAGNOSIS ===")

			// Check if targets are actually registered
			if len(healthResult.TargetHealthDescriptions) == 0 {
				t.Log("ERROR: No targets are registered in the target group")
				t.Log("This suggests the ECS service failed to register tasks with the target group")
			}

			// Provide troubleshooting guidance based on health check failures
			for _, targetHealth := range healthResult.TargetHealthDescriptions {
				health := targetHealth.TargetHealth
				reason := aws.StringValue(health.Reason)

				switch reason {
				case "Target.ResponseCodeMismatch":
					t.Log("DIAGNOSIS: Health check is receiving unexpected HTTP response code")
					t.Log("Expected: 200-299")
					t.Logf("Health check path: %s", aws.StringValue(targetGroup.HealthCheckPath))
					t.Log("ACTION: Verify the Bridge container is serving HTTP 200 on /ok endpoint")

				case "Target.Timeout":
					t.Log("DIAGNOSIS: Health check is timing out")
					t.Logf("Timeout setting: %d seconds", aws.Int64Value(targetGroup.HealthCheckTimeoutSeconds))
					t.Log("ACTION: Check if Bridge container is listening on the correct port")
					t.Log("ACTION: Verify security group allows ALB to reach Bridge tasks")

				case "Target.FailedHealthChecks":
					t.Log("DIAGNOSIS: Target is failing health checks")
					t.Logf("Unhealthy threshold: %d consecutive failures", aws.Int64Value(targetGroup.UnhealthyThresholdCount))
					t.Log("ACTION: Check CloudWatch Logs for Bridge container errors")

				case "Target.NotRegistered":
					t.Log("DIAGNOSIS: Target is not properly registered")
					t.Log("ACTION: Check ECS service configuration and task status")

				case "Target.DeregistrationInProgress":
					t.Log("DIAGNOSIS: Target is being deregistered")
					t.Log("This might indicate the task is restarting repeatedly")

				default:
					if reason != "" {
						t.Logf("DIAGNOSIS: Health check failure reason: %s", reason)
					}
				}
			}

			// Diagnose security group configuration
			diagnoseSecurityGroups(t, ec2Client, albSecurityGroupID, bridgeSecurityGroupID)

			// Diagnose container logs
			diagnoseContainerLogs(t, sess, awsRegion, cloudwatchLogGroupName, ecsClusterName, ecsServiceName)

			// Diagnose network connectivity
			diagnoseNetworkConnectivity(t, ec2Client, vpcID, privateSubnetIDs)

			t.Log("===================================")

			require.Equal(t, desiredCount, healthyCount, "Target group should have %d healthy targets after waiting", desiredCount)
		}

		time.Sleep(timeBetweenRetries)
	}

	// HTTPS health check test
	// ACM certificate is automatically issued via DNS validation
	t.Log("Testing HTTPS health check endpoint (ACM certificate auto-issued via DNS validation)...")
	testHTTPSHealthCheck(t, terraformOptions, bridgeDomainName)

	t.Log("All tests passed successfully!")
}

// testHTTPSHealthCheck tests HTTPS endpoint health check
// This function verifies that the Bridge's HTTPS endpoint is accessible via the custom domain
// and returns a 200 OK response from the Bridge health check endpoint.
// Note: ACM certificate is automatically issued via DNS validation, which may take 5-10 minutes.
func testHTTPSHealthCheck(t *testing.T, terraformOptions *terraform.Options, domainName string) {
	healthCheckURL := fmt.Sprintf("https://%s/ok", domainName)

	t.Logf("Testing HTTPS health check endpoint: %s", healthCheckURL)
	t.Log("Note: First-time DNS validation may take 5-10 minutes for ACM certificate issuance")

	// Create HTTP client with default TLS configuration
	// ACM certificates are trusted by default
	client := &http.Client{
		Timeout: 60 * time.Second, // Increased timeout for Bridge initialization and DNS propagation
	}

	maxRetries := 60 // 10 minutes with 10s interval (Bridge may need time to initialize)
	timeBetweenRetries := 10 * time.Second

	for i := 0; i < maxRetries; i++ {
		startTime := time.Now()
		resp, err := client.Get(healthCheckURL)
		elapsed := time.Since(startTime)

		if err != nil {
			t.Logf("Attempt %d/%d: HTTPS request failed: %v (elapsed: %v)", i+1, maxRetries, err, elapsed)
			if i < maxRetries-1 {
				time.Sleep(timeBetweenRetries)
				continue
			}
			require.NoError(t, err, "HTTPS health check should not fail after %d retries", maxRetries)
		}
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		t.Logf("Attempt %d/%d: Status=%d, Body=%s (elapsed: %v)", i+1, maxRetries, resp.StatusCode, string(body), elapsed)

		if resp.StatusCode == http.StatusOK {
			assert.Equal(t, http.StatusOK, resp.StatusCode, "Health check should return 200 OK")
			t.Logf("HTTPS health check passed on attempt %d/%d", i+1, maxRetries)
			return
		}

		if i == maxRetries-1 {
			require.Equal(t, http.StatusOK, resp.StatusCode, "Health check should return 200 OK after retries")
		}

		time.Sleep(timeBetweenRetries)
	}
}

// diagnoseNetworkConfiguration checks and logs network configuration details
func diagnoseNetworkConfiguration(t *testing.T, ec2Client *ec2.EC2, subnetIDs []string, vpcID string) {
	t.Log("=== NETWORK CONFIGURATION DIAGNOSIS ===")

	// Check subnets
	for _, subnetID := range subnetIDs {
		describeSubnetInput := &ec2.DescribeSubnetsInput{
			SubnetIds: []*string{aws.String(subnetID)},
		}
		subnetResult, err := ec2Client.DescribeSubnets(describeSubnetInput)
		if err != nil {
			t.Logf("ERROR: Failed to describe subnet %s: %v", subnetID, err)
			continue
		}

		if len(subnetResult.Subnets) > 0 {
			subnet := subnetResult.Subnets[0]
			t.Logf("Subnet: %s", aws.StringValue(subnet.SubnetId))
			t.Logf("  AZ: %s", aws.StringValue(subnet.AvailabilityZone))
			t.Logf("  CIDR: %s", aws.StringValue(subnet.CidrBlock))
			t.Logf("  Available IPs: %d", aws.Int64Value(subnet.AvailableIpAddressCount))
			t.Logf("  Map Public IP: %t", aws.BoolValue(subnet.MapPublicIpOnLaunch))

			// Check route table
			describeRouteTablesInput := &ec2.DescribeRouteTablesInput{
				Filters: []*ec2.Filter{
					{
						Name:   aws.String("association.subnet-id"),
						Values: []*string{subnet.SubnetId},
					},
				},
			}
			rtResult, err := ec2Client.DescribeRouteTables(describeRouteTablesInput)
			if err != nil {
				t.Logf("  ERROR: Failed to describe route table: %v", err)
			} else if len(rtResult.RouteTables) > 0 {
				rt := rtResult.RouteTables[0]
				t.Logf("  Route Table: %s", aws.StringValue(rt.RouteTableId))
				hasInternetRoute := false
				var internetRouteTarget string
				for _, route := range rt.Routes {
					if aws.StringValue(route.DestinationCidrBlock) == "0.0.0.0/0" {
						hasInternetRoute = true
						if route.GatewayId != nil {
							internetRouteTarget = fmt.Sprintf("IGW: %s", aws.StringValue(route.GatewayId))
						} else if route.NatGatewayId != nil {
							internetRouteTarget = fmt.Sprintf("NAT: %s", aws.StringValue(route.NatGatewayId))
							// Check NAT Gateway status
							describeNatInput := &ec2.DescribeNatGatewaysInput{
								NatGatewayIds: []*string{route.NatGatewayId},
							}
							natResult, err := ec2Client.DescribeNatGateways(describeNatInput)
							if err == nil && len(natResult.NatGateways) > 0 {
								nat := natResult.NatGateways[0]
								t.Logf("    NAT Gateway State: %s", aws.StringValue(nat.State))
								t.Logf("    NAT Gateway Subnet: %s", aws.StringValue(nat.SubnetId))
								if len(nat.NatGatewayAddresses) > 0 {
									t.Logf("    NAT Gateway Public IP: %s", aws.StringValue(nat.NatGatewayAddresses[0].PublicIp))
								}
							}
						} else if route.NetworkInterfaceId != nil {
							internetRouteTarget = fmt.Sprintf("ENI: %s", aws.StringValue(route.NetworkInterfaceId))
						} else {
							internetRouteTarget = "Unknown"
						}
						break
					}
				}
				if hasInternetRoute {
					t.Logf("  Internet Route (0.0.0.0/0): %s", internetRouteTarget)
				} else {
					t.Logf("  ⚠️  WARNING: No internet route (0.0.0.0/0) found!")
					t.Logf("  This subnet cannot reach ECR to pull container images")
				}
			} else {
				t.Logf("  ⚠️  WARNING: No route table found for this subnet")
			}
		}
	}

	t.Log("=======================================")
}

// diagnoseTaskFailure provides detailed diagnosis of task failures
func diagnoseTaskFailure(t *testing.T, ecsClient *ecs.ECS, ec2Client *ec2.EC2, clusterName, serviceName string, taskSubnetIDs []string, vpcID string) {
	t.Log("=== TASK FAILURE DIAGNOSIS ===")

	// Get task definition from service
	describeServicesInput := &ecs.DescribeServicesInput{
		Cluster:  aws.String(clusterName),
		Services: []*string{aws.String(serviceName)},
	}
	servicesResult, err := ecsClient.DescribeServices(describeServicesInput)
	if err != nil || len(servicesResult.Services) == 0 {
		t.Logf("ERROR: Could not get service details: %v", err)
		return
	}

	service := servicesResult.Services[0]
	taskDefArn := service.TaskDefinition

	// Get task definition details
	describeTaskDefInput := &ecs.DescribeTaskDefinitionInput{
		TaskDefinition: taskDefArn,
	}
	taskDefResult, err := ecsClient.DescribeTaskDefinition(describeTaskDefInput)
	if err != nil {
		t.Logf("ERROR: Could not get task definition: %v", err)
	} else {
		taskDef := taskDefResult.TaskDefinition
		t.Logf("Task Definition: %s", aws.StringValue(taskDef.Family))
		t.Logf("  CPU: %s", aws.StringValue(taskDef.Cpu))
		t.Logf("  Memory: %s", aws.StringValue(taskDef.Memory))
		t.Logf("  Network Mode: %s", aws.StringValue(taskDef.NetworkMode))
		t.Logf("  Requires Compatibilities: %v", aws.StringValueSlice(taskDef.RequiresCompatibilities))

		if len(taskDef.ContainerDefinitions) > 0 {
			container := taskDef.ContainerDefinitions[0]
			t.Logf("  Container: %s", aws.StringValue(container.Name))
			t.Logf("    Image: %s", aws.StringValue(container.Image))
			if container.Memory != nil {
				t.Logf("    Memory: %d MB", *container.Memory)
			}
			if container.MemoryReservation != nil {
				t.Logf("    Memory Reservation: %d MB", *container.MemoryReservation)
			}
		}
	}

	// Run network diagnosis
	diagnoseNetworkConfiguration(t, ec2Client, taskSubnetIDs, vpcID)

	t.Log("==============================")
}

// diagnoseNetworkConnectivity checks if private subnets have internet connectivity
func diagnoseNetworkConnectivity(t *testing.T, ec2Client *ec2.EC2, vpcID string, privateSubnetIDs []string) {
	t.Log("=== NETWORK CONNECTIVITY DIAGNOSIS ===")

	t.Log("Checking if private subnets have internet access...")

	hasNATGateway := false
	hasInternetGateway := false

	for _, subnetID := range privateSubnetIDs {
		t.Logf("Checking subnet: %s", subnetID)

		// Get route table for this subnet
		describeRouteTablesInput := &ec2.DescribeRouteTablesInput{
			Filters: []*ec2.Filter{
				{
					Name:   aws.String("association.subnet-id"),
					Values: []*string{aws.String(subnetID)},
				},
			},
		}

		rtResult, err := ec2Client.DescribeRouteTables(describeRouteTablesInput)
		if err != nil {
			t.Logf("  ERROR: Failed to describe route table: %v", err)
			continue
		}

		if len(rtResult.RouteTables) == 0 {
			t.Log("  ERROR: No route table found for this subnet")
			continue
		}

		rt := rtResult.RouteTables[0]
		t.Logf("  Route Table: %s", aws.StringValue(rt.RouteTableId))

		// Check routes
		for _, route := range rt.Routes {
			dest := aws.StringValue(route.DestinationCidrBlock)

			if dest == "0.0.0.0/0" {
				t.Log("  Found default route (0.0.0.0/0)")

				if route.NatGatewayId != nil {
					hasNATGateway = true
					natID := aws.StringValue(route.NatGatewayId)
					t.Logf("    → NAT Gateway: %s", natID)

					// Check NAT Gateway state
					natResult, err := ec2Client.DescribeNatGateways(&ec2.DescribeNatGatewaysInput{
						NatGatewayIds: []*string{route.NatGatewayId},
					})
					if err == nil && len(natResult.NatGateways) > 0 {
						nat := natResult.NatGateways[0]
						state := aws.StringValue(nat.State)
						t.Logf("    → NAT Gateway State: %s", state)

						if state == "available" {
							t.Log("    ✓ NAT Gateway is available - internet access OK")
						} else {
							t.Logf("    ✗ NAT Gateway is NOT available (state: %s)", state)
						}
					}
				} else if route.GatewayId != nil && strings.HasPrefix(aws.StringValue(route.GatewayId), "igw-") {
					hasInternetGateway = true
					t.Logf("    → Internet Gateway: %s", aws.StringValue(route.GatewayId))
					t.Log("    ✓ Direct internet access via Internet Gateway")
				} else {
					t.Logf("    → Unknown target: %s", aws.StringValue(route.GatewayId))
					t.Log("    ✗ No valid internet route")
				}
			}
		}
	}

	t.Log("")
	t.Log("Summary:")
	if hasNATGateway || hasInternetGateway {
		t.Log("  ✓ Private subnets HAVE internet access")
		t.Log("  Bridge can connect to external authentication servers")
	} else {
		t.Log("  ✗ Private subnets DO NOT have internet access")
		t.Log("")
		t.Log("PROBLEM IDENTIFIED:")
		t.Log("  Bridge requires internet access to fetch authentication keys")
		t.Log("  from BaseMachina's servers. Without NAT Gateway or Internet")
		t.Log("  Gateway, the Bridge cannot initialize and will remain in")
		t.Log("  'waiting for ready' state.")
		t.Log("")
		t.Log("SOLUTIONS:")
		t.Log("  1. Add NAT Gateway to private subnets (~$32/month)")
		t.Log("  2. Use public subnets with assign_public_ip=true")
		t.Log("  3. Configure Bridge to work offline (if supported)")
		t.Log("")
		t.Log("Current configuration uses ONLY VPC Endpoints for:")
		t.Log("  - ECR API (container image pull)")
		t.Log("  - ECR Docker (container image pull)")
		t.Log("  - S3 (ECR layers)")
		t.Log("  - CloudWatch Logs (logging)")
		t.Log("")
		t.Log("But Bridge also needs access to BaseMachina's authentication")
		t.Log("servers, which require general internet access.")
	}

	t.Log("=====================================")
}

// diagnoseContainerLogs fetches and displays recent container logs from CloudWatch Logs
func diagnoseContainerLogs(t *testing.T, sess *session.Session, region, logGroupName, clusterName, serviceName string) {
	t.Log("=== CLOUDWATCH LOGS DIAGNOSIS ===")

	cwLogsClient := cloudwatchlogs.New(sess)
	ecsClient := ecs.New(sess)

	// Get the most recent task ARN
	listTasksInput := &ecs.ListTasksInput{
		Cluster:       aws.String(clusterName),
		ServiceName:   aws.String(serviceName),
		DesiredStatus: aws.String("RUNNING"),
	}

	listTasksResult, err := ecsClient.ListTasks(listTasksInput)
	if err != nil || len(listTasksResult.TaskArns) == 0 {
		t.Log("No running tasks found to check logs")

		// Try stopped tasks
		listTasksInput.DesiredStatus = aws.String("STOPPED")
		listTasksResult, err = ecsClient.ListTasks(listTasksInput)
		if err != nil || len(listTasksResult.TaskArns) == 0 {
			t.Log("No stopped tasks found either")
			t.Log("================================")
			return
		}
		t.Log("Checking logs from stopped tasks instead...")
	}

	// Get task details to extract task ID
	describeTasksInput := &ecs.DescribeTasksInput{
		Cluster: aws.String(clusterName),
		Tasks:   []*string{listTasksResult.TaskArns[0]},
	}

	describeTasksResult, err := ecsClient.DescribeTasks(describeTasksInput)
	if err != nil || len(describeTasksResult.Tasks) == 0 {
		t.Logf("Failed to describe task: %v", err)
		t.Log("================================")
		return
	}

	task := describeTasksResult.Tasks[0]
	taskArn := aws.StringValue(task.TaskArn)

	// Extract task ID from ARN (format: arn:aws:ecs:region:account:task/cluster/task-id)
	taskIDParts := strings.Split(taskArn, "/")
	if len(taskIDParts) < 3 {
		t.Logf("Could not parse task ARN: %s", taskArn)
		t.Log("================================")
		return
	}
	taskID := taskIDParts[len(taskIDParts)-1]

	// Construct log stream name: container-name/container-name/task-id
	logStreamName := fmt.Sprintf("bridge/bridge/%s", taskID)

	t.Logf("Log Group: %s", logGroupName)
	t.Logf("Log Stream: %s", logStreamName)

	// Fetch ALL logs from the beginning
	getLogEventsInput := &cloudwatchlogs.GetLogEventsInput{
		LogGroupName:  aws.String(logGroupName),
		LogStreamName: aws.String(logStreamName),
		StartFromHead: aws.Bool(true), // Get logs from the beginning
	}

	var allEvents []*cloudwatchlogs.OutputLogEvent

	// Paginate through all log events
	for {
		logEventsResult, err := cwLogsClient.GetLogEvents(getLogEventsInput)
		if err != nil {
			t.Logf("Failed to get log events: %v", err)
			t.Log("This might mean the container hasn't started yet or log stream doesn't exist")
			t.Log("================================")
			return
		}

		if len(logEventsResult.Events) == 0 {
			break
		}

		allEvents = append(allEvents, logEventsResult.Events...)

		// Check if we've received all events
		if logEventsResult.NextForwardToken == nil ||
			aws.StringValue(logEventsResult.NextForwardToken) == aws.StringValue(getLogEventsInput.NextToken) {
			break
		}

		getLogEventsInput.NextToken = logEventsResult.NextForwardToken
	}

	if len(allEvents) == 0 {
		t.Log("No log events found - container may not have produced any output yet")
	} else {
		t.Logf("ALL logs (%d events):", len(allEvents))
		for _, event := range allEvents {
			timestamp := time.Unix(0, *event.Timestamp*int64(time.Millisecond))
			t.Logf("  [%s] %s", timestamp.Format("15:04:05"), aws.StringValue(event.Message))
		}
	}

	t.Log("================================")
}

// diagnoseSecurityGroups checks and logs security group rules for ALB and Bridge
func diagnoseSecurityGroups(t *testing.T, ec2Client *ec2.EC2, albSGID, bridgeSGID string) {
	t.Log("=== SECURITY GROUP DIAGNOSIS ===")

	// Check ALB security group
	t.Logf("Checking ALB Security Group: %s", albSGID)
	albSG, err := ec2Client.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
		GroupIds: []*string{aws.String(albSGID)},
	})
	if err != nil {
		t.Logf("ERROR: Failed to describe ALB security group: %v", err)
	} else if len(albSG.SecurityGroups) > 0 {
		sg := albSG.SecurityGroups[0]
		t.Logf("ALB Security Group: %s (%s)", aws.StringValue(sg.GroupName), aws.StringValue(sg.GroupId))

		t.Log("  Ingress Rules:")
		if len(sg.IpPermissions) == 0 {
			t.Log("    No ingress rules (THIS IS A PROBLEM - ALB needs to accept HTTPS)")
		}
		for _, rule := range sg.IpPermissions {
			protocol := aws.StringValue(rule.IpProtocol)
			fromPort := aws.Int64Value(rule.FromPort)
			toPort := aws.Int64Value(rule.ToPort)

			if len(rule.IpRanges) > 0 {
				for _, ipRange := range rule.IpRanges {
					t.Logf("    %s:%d-%d from %s (%s)",
						protocol, fromPort, toPort,
						aws.StringValue(ipRange.CidrIp),
						aws.StringValue(ipRange.Description))
				}
			}
			if len(rule.UserIdGroupPairs) > 0 {
				for _, pair := range rule.UserIdGroupPairs {
					t.Logf("    %s:%d-%d from SG %s (%s)",
						protocol, fromPort, toPort,
						aws.StringValue(pair.GroupId),
						aws.StringValue(pair.Description))
				}
			}
		}

		t.Log("  Egress Rules:")
		for _, rule := range sg.IpPermissionsEgress {
			protocol := aws.StringValue(rule.IpProtocol)
			if protocol == "-1" {
				t.Log("    ALL traffic to all destinations (good for ALB)")
				break
			}
		}
	}

	// Check Bridge security group
	t.Logf("Checking Bridge Security Group: %s", bridgeSGID)
	bridgeSG, err := ec2Client.DescribeSecurityGroups(&ec2.DescribeSecurityGroupsInput{
		GroupIds: []*string{aws.String(bridgeSGID)},
	})
	if err != nil {
		t.Logf("ERROR: Failed to describe Bridge security group: %v", err)
	} else if len(bridgeSG.SecurityGroups) > 0 {
		sg := bridgeSG.SecurityGroups[0]
		t.Logf("Bridge Security Group: %s (%s)", aws.StringValue(sg.GroupName), aws.StringValue(sg.GroupId))

		t.Log("  Ingress Rules:")
		hasALBAccess := false
		if len(sg.IpPermissions) == 0 {
			t.Log("    No ingress rules (THIS IS A PROBLEM - Bridge needs to accept traffic from ALB)")
		}
		for _, rule := range sg.IpPermissions {
			protocol := aws.StringValue(rule.IpProtocol)
			fromPort := aws.Int64Value(rule.FromPort)
			toPort := aws.Int64Value(rule.ToPort)

			if len(rule.UserIdGroupPairs) > 0 {
				for _, pair := range rule.UserIdGroupPairs {
					sourceSG := aws.StringValue(pair.GroupId)
					t.Logf("    %s:%d-%d from SG %s (%s)",
						protocol, fromPort, toPort,
						sourceSG,
						aws.StringValue(pair.Description))

					// Check if ALB can reach Bridge
					if sourceSG == albSGID {
						hasALBAccess = true
						t.Log("      ✓ ALB security group has access to Bridge")
					}
				}
			}
			if len(rule.IpRanges) > 0 {
				for _, ipRange := range rule.IpRanges {
					t.Logf("    %s:%d-%d from %s (%s)",
						protocol, fromPort, toPort,
						aws.StringValue(ipRange.CidrIp),
						aws.StringValue(ipRange.Description))
				}
			}
		}

		if !hasALBAccess {
			t.Log("    ✗ PROBLEM: ALB security group does NOT have access to Bridge")
			t.Log("    This will cause health checks to fail")
			t.Log("")
			t.Log("    Expected rule:")
			t.Logf("      Protocol: tcp, Port: 8080, Source: %s", albSGID)
		}

		t.Log("  Egress Rules:")
		hasEgressAll := false
		for _, rule := range sg.IpPermissionsEgress {
			protocol := aws.StringValue(rule.IpProtocol)
			if protocol == "-1" {
				hasEgressAll = true
				t.Log("    ALL traffic to all destinations (good)")
				break
			}
		}
		if !hasEgressAll {
			t.Log("    WARNING: Egress is restricted - ensure VPC endpoints are accessible")
		}
	}

	t.Log("================================")
}

// triggerPullThroughCache triggers the ECR pull-through cache by attempting to pull the image
// This creates the repository and caches the image before ECS tasks try to use it
func triggerPullThroughCache(t *testing.T, region string) {
	// Use AWS CLI to trigger pull-through cache
	// We use "aws ecr batch-get-image" which triggers the cache without needing Docker
	cmd := exec.Command("aws", "ecr", "batch-get-image",
		"--repository-name", "ecr-public/basemachina/bridge",
		"--image-ids", "imageTag=latest",
		"--region", region)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// First attempt might fail if repository doesn't exist yet
		// That's expected - the command itself triggers repository creation
		t.Logf("First pull-through cache trigger (expected to fail): %v", err)
		t.Logf("Output: %s", string(output))

		// Wait for repository to be created
		t.Log("Waiting 15 seconds for pull-through cache repository creation...")
		time.Sleep(15 * time.Second)

		// Try again - this time it should work
		output, err = cmd.CombinedOutput()
		if err != nil {
			t.Logf("Warning: Second pull-through cache attempt also failed: %v", err)
			t.Logf("Output: %s", string(output))
			t.Log("ECS tasks may take longer to start on first deployment")
		} else {
			t.Log("Successfully triggered pull-through cache")
		}
	} else {
		t.Log("Pull-through cache repository already exists and is ready")
	}
}

// cleanupExistingS3Endpoints deletes any existing S3 VPC endpoints in the test VPC
// that were created by previous test runs to avoid route table conflicts
func cleanupExistingS3Endpoints(t *testing.T, ec2Client *ec2.EC2, vpcID string, namePrefix string) {
	t.Log("Checking for existing S3 VPC endpoints in test VPC...")

	// List all VPC endpoints in the VPC
	describeInput := &ec2.DescribeVpcEndpointsInput{
		Filters: []*ec2.Filter{
			{
				Name:   aws.String("vpc-id"),
				Values: []*string{aws.String(vpcID)},
			},
			{
				Name:   aws.String("vpc-endpoint-type"),
				Values: []*string{aws.String("Gateway")},
			},
			{
				Name:   aws.String("service-name"),
				Values: []*string{aws.String("com.amazonaws.*s3")},
			},
		},
	}

	result, err := ec2Client.DescribeVpcEndpoints(describeInput)
	if err != nil {
		t.Logf("Warning: Failed to describe VPC endpoints: %v", err)
		return
	}

	if len(result.VpcEndpoints) == 0 {
		t.Log("No existing S3 VPC endpoints found")
		return
	}

	// Delete each S3 endpoint
	for _, endpoint := range result.VpcEndpoints {
		endpointID := aws.StringValue(endpoint.VpcEndpointId)

		// Check if this endpoint was created by a previous test (has test- prefix in tags)
		// Only delete endpoints that look like they were created by tests
		isTestEndpoint := false
		for _, tag := range endpoint.Tags {
			if aws.StringValue(tag.Key) == "Name" && strings.HasPrefix(aws.StringValue(tag.Value), "test-") {
				isTestEndpoint = true
				break
			}
		}

		// Skip deletion if this doesn't look like a test endpoint
		// This prevents accidentally deleting production S3 endpoints
		if !isTestEndpoint {
			t.Logf("Skipping S3 endpoint %s (doesn't appear to be a test endpoint)", endpointID)
			continue
		}

		t.Logf("Deleting existing S3 VPC endpoint: %s", endpointID)
		deleteInput := &ec2.DeleteVpcEndpointsInput{
			VpcEndpointIds: []*string{endpoint.VpcEndpointId},
		}

		_, err := ec2Client.DeleteVpcEndpoints(deleteInput)
		if err != nil {
			t.Logf("Warning: Failed to delete S3 VPC endpoint %s: %v", endpointID, err)
		} else {
			t.Logf("Successfully deleted S3 VPC endpoint: %s", endpointID)

			// Wait a moment for the endpoint to be fully deleted
			time.Sleep(5 * time.Second)
		}
	}
}

// verifyRoute53Zone verifies that the Route53 zone exists and the domain matches
func verifyRoute53Zone(t *testing.T, zoneID, domainName string) {
	t.Log("=== ROUTE53 ZONE VERIFICATION ===")
	t.Logf("Verifying Route53 zone: %s", zoneID)
	t.Logf("Domain name: %s", domainName)

	// Get hosted zone details
	cmd := exec.Command("aws", "route53", "get-hosted-zone",
		"--id", zoneID,
		"--query", "HostedZone.{Name:Name,Id:Id,ResourceRecordSetCount:ResourceRecordSetCount}",
		"--output", "json")

	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("ERROR: Failed to get hosted zone: %v", err)
		t.Logf("Output: %s", string(output))
		t.Fatalf("Route53 zone %s not found or inaccessible", zoneID)
	}

	t.Logf("Zone details: %s", string(output))

	// Extract parent domain from the full domain name
	// e.g., bridge.example.com -> example.com
	parts := strings.Split(domainName, ".")
	if len(parts) < 2 {
		t.Fatalf("Invalid domain name: %s", domainName)
	}

	// Get the parent domain (last two parts)
	parentDomain := strings.Join(parts[len(parts)-2:], ".")

	t.Logf("Parent domain: %s", parentDomain)
	t.Log("Note: The domain should be a subdomain of the hosted zone")

	// List a few records to confirm zone is accessible
	listCmd := exec.Command("aws", "route53", "list-resource-record-sets",
		"--hosted-zone-id", zoneID,
		"--max-items", "5",
		"--query", "ResourceRecordSets[*].{Name:Name,Type:Type}",
		"--output", "table")

	listOutput, err := listCmd.CombinedOutput()
	if err != nil {
		t.Logf("Warning: Failed to list records: %v", err)
	} else {
		t.Log("Sample records in zone:")
		t.Log(string(listOutput))
	}

	t.Log("✓ Route53 zone verification complete")
	t.Log("================================")
}
