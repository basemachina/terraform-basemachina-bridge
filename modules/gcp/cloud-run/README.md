# Google Cloud Run Module for BaseMachina Bridge

このTerraformモジュールは、BaseMachina BridgeをGoogle Cloud Run上にデプロイします。Bridgeは、BaseMachinaからお客様のデータベースやAPIへアクセスする際に中継する、認証機能付きのゲートウェイ（プロキシ）です。

## 概要

このモジュールは以下のリソースを作成します：

- **Cloud Run v2 Service**: Bridgeコンテナを実行するサーバーレスコンピューティング環境
- **Service Account**: Cloud Runサービス用の専用サービスアカウント（最小権限）
- **Cloud Load Balancer**: HTTPS/HTTPトラフィックのルーティング（オプション）
- **Google-managed SSL Certificate**: カスタムドメイン用の自動SSL証明書（オプション）
- **Cloud Armor**: IPベースのアクセス制御（オプション）
- **Cloud DNS**: ドメイン名のDNSレコード（オプション）
- **VPC Integration**: Direct VPC EgressまたはVPC Connectorによるプライベートネットワーク接続

## 前提条件

このモジュールを使用する前に、以下の準備が必要です：

1. **Google Cloud Project**: 有効なGoogle CloudプロジェクトIDを用意
2. **APIの有効化**: 以下のAPIを有効にする必要があります
   ```bash
   gcloud services enable run.googleapis.com
   gcloud services enable compute.googleapis.com
   gcloud services enable dns.googleapis.com  # DNS統合を使用する場合
   ```
3. **VPCネットワーク**: Cloud SQLやその他のプライベートリソースに接続する場合、VPCネットワークとサブネットを事前に作成
4. **Terraform**: バージョン1.5以上
5. **Google Cloud Provider**: バージョン5.0以上
6. **Tenant ID**: BaseMachinaから提供されるテナントID

## 使用例

### 基本的な使用例（HTTPのみ）

```hcl
module "basemachina_bridge" {
  source = "../../modules/gcp/cloud-run"

  project_id  = "my-gcp-project"
  region      = "asia-northeast1"
  tenant_id   = "your-tenant-id"

  service_name    = "basemachina-bridge"
  fetch_interval  = "1h"
  fetch_timeout   = "10s"
  port            = 8080
}
```

### HTTPS対応（カスタムドメイン + SSL証明書）

```hcl
module "basemachina_bridge" {
  source = "../../modules/gcp/cloud-run"

  project_id  = "my-gcp-project"
  region      = "asia-northeast1"
  tenant_id   = "your-tenant-id"

  # カスタムドメインとHTTPS設定
  domain_name            = "bridge.example.com"
  enable_https_redirect  = true
  enable_cloud_armor     = true
  allowed_ip_ranges      = ["34.85.43.93/32"]

  # Cloud DNS統合（オプション）
  dns_zone_name = "example-com"
}
```

### VPC統合（Direct VPC Egress）

```hcl
module "basemachina_bridge" {
  source = "../../modules/gcp/cloud-run"

  project_id  = "my-gcp-project"
  region      = "asia-northeast1"
  tenant_id   = "your-tenant-id"

  domain_name = "bridge.example.com"

  # Direct VPC Egress（推奨）
  vpc_network_id    = "projects/my-project/global/networks/my-vpc"
  vpc_subnetwork_id = "projects/my-project/regions/asia-northeast1/subnetworks/my-subnet"
  vpc_egress        = "PRIVATE_RANGES_ONLY"
}
```

### リソース設定のカスタマイズ

```hcl
module "basemachina_bridge" {
  source = "../../modules/gcp/cloud-run"

  project_id  = "my-gcp-project"
  region      = "asia-northeast1"
  tenant_id   = "your-tenant-id"

  domain_name = "bridge.example.com"

  # リソース設定
  cpu          = "2"
  memory       = "1Gi"
  min_instances = 1
  max_instances = 20
  # bridge_image_tag = "v1.0.0"  # オプション: 特定バージョンを指定（デフォルト: latest）

  # ラベル
  labels = {
    environment = "production"
    team        = "data-platform"
  }
}
```

## 出力値の使用

```hcl
# Cloud Run service URL
output "bridge_url" {
  value = module.basemachina_bridge.service_url
}

# Load Balancer IP（カスタムドメインを使用する場合）
output "bridge_ip" {
  value = module.basemachina_bridge.load_balancer_ip
}

# サービスアカウント
output "bridge_sa" {
  value = module.basemachina_bridge.service_account_email
}
```

## セキュリティ考慮事項

- **最小権限の原則**: サービスアカウントには必要最小限の権限（Cloud SQL Client、Log Writer）のみを付与
- **IPホワイトリスト**: Cloud Armorを使用したIPベースのアクセス制御
  - **BaseMachina IP自動追加**: 34.85.43.93/32 は `allowed_ip_ranges` に明示的に指定しなくても自動的に許可されます
  - **追加IP指定**: `allowed_ip_ranges` で追加のIPアドレス範囲を指定できます（デフォルト: 空リスト `[]`）
  - **全IP許可**: `allowed_ip_ranges = ["*"]` を指定すると全てのIPアドレスからアクセス可能になります
- **HTTPS強制**: `enable_https_redirect = true`でHTTPトラフィックを自動的にHTTPSにリダイレクト
- **プライベートネットワーク**: Cloud Runサービスは内部ロードバランサー経由でのみアクセス可能（`INGRESS_TRAFFIC_INTERNAL_LOAD_BALANCER`）

## トラブルシューティング

### SSL証明書のプロビジョニングが完了しない

Google-managed SSL証明書の発行には、DNSレコードが正しく設定され、伝播が完了している必要があります（最大15分）。`dns_zone_name`を指定している場合、Aレコードは自動的に作成されます。

### VPC接続エラー

Direct VPC Egressを使用する場合、以下を確認してください：
- VPCネットワークとサブネットが同じリージョンに存在すること
- Cloud RunサービスアカウントにVPCへのアクセス権限があること

### Cloud Armorによるアクセス拒否

`allowed_ip_ranges`に自分のIPアドレスが含まれていることを確認してください。デフォルトではBaseMachinaのIP（34.85.43.93/32）のみが許可されています。

<!-- BEGIN_TF_DOCS -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.5 |
| <a name="requirement_google"></a> [google](#requirement\_google) | ~> 5.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | 5.45.2 |

## Modules

No modules.

## Resources

| Name | Type |
|------|------|
| [google_cloud_run_v2_service.bridge](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloud_run_v2_service) | resource |
| [google_cloud_run_v2_service_iam_member.invoker](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/cloud_run_v2_service_iam_member) | resource |
| [google_compute_backend_service.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_backend_service) | resource |
| [google_compute_global_address.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_global_address) | resource |
| [google_compute_global_forwarding_rule.http](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_global_forwarding_rule) | resource |
| [google_compute_global_forwarding_rule.https](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_global_forwarding_rule) | resource |
| [google_compute_managed_ssl_certificate.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_managed_ssl_certificate) | resource |
| [google_compute_region_network_endpoint_group.cloud_run_neg](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_region_network_endpoint_group) | resource |
| [google_compute_security_policy.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_security_policy) | resource |
| [google_compute_target_http_proxy.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_target_http_proxy) | resource |
| [google_compute_target_https_proxy.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_target_https_proxy) | resource |
| [google_compute_url_map.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_url_map) | resource |
| [google_compute_url_map.https_redirect](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_url_map) | resource |
| [google_dns_record_set.default](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/dns_record_set) | resource |
| [google_project_iam_member.cloud_sql_client](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_iam_member) | resource |
| [google_project_iam_member.log_writer](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/project_iam_member) | resource |
| [google_service_account.bridge](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/service_account) | resource |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_allowed_ip_ranges"></a> [allowed\_ip\_ranges](#input\_allowed\_ip\_ranges) | Additional IP ranges allowed to access the service. BaseMachina IP (34.85.43.93/32) is automatically included unless '*' is specified to allow all IPs. | `list(string)` | `[]` | no |
| <a name="input_bridge_image_tag"></a> [bridge\_image\_tag](#input\_bridge\_image\_tag) | Bridge container image tag (default: latest). Specify a specific version like 'v1.0.0' if needed. | `string` | `"latest"` | no |
| <a name="input_cpu"></a> [cpu](#input\_cpu) | CPU allocation for Cloud Run service (e.g., '1', '2', '4') | `string` | `"1"` | no |
| <a name="input_dns_zone_name"></a> [dns\_zone\_name](#input\_dns\_zone\_name) | Cloud DNS Managed Zone name (optional, required for DNS record creation) | `string` | `null` | no |
| <a name="input_domain_name"></a> [domain\_name](#input\_domain\_name) | Custom domain name for the Bridge (optional, required for HTTPS) | `string` | `null` | no |
| <a name="input_enable_cloud_armor"></a> [enable\_cloud\_armor](#input\_enable\_cloud\_armor) | Enable Cloud Armor security policy | `bool` | `true` | no |
| <a name="input_enable_https_redirect"></a> [enable\_https\_redirect](#input\_enable\_https\_redirect) | Enable HTTP to HTTPS redirect | `bool` | `true` | no |
| <a name="input_fetch_interval"></a> [fetch\_interval](#input\_fetch\_interval) | Interval for fetching public keys (e.g., 1h, 30m) | `string` | `"1h"` | no |
| <a name="input_fetch_timeout"></a> [fetch\_timeout](#input\_fetch\_timeout) | Timeout for fetching public keys (e.g., 10s, 30s) | `string` | `"10s"` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to apply to all resources | `map(string)` | `{}` | no |
| <a name="input_max_instances"></a> [max\_instances](#input\_max\_instances) | Maximum number of instances | `number` | `10` | no |
| <a name="input_memory"></a> [memory](#input\_memory) | Memory allocation for Cloud Run service (e.g., '512Mi', '1Gi', '2Gi') | `string` | `"512Mi"` | no |
| <a name="input_min_instances"></a> [min\_instances](#input\_min\_instances) | Minimum number of instances | `number` | `0` | no |
| <a name="input_port"></a> [port](#input\_port) | Container port number (cannot be 4321). Cloud Run automatically sets PORT environment variable to this value. | `number` | `8080` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Google Cloud Project ID where resources will be created | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Google Cloud region for Cloud Run service | `string` | `"asia-northeast1"` | no |
| <a name="input_service_name"></a> [service\_name](#input\_service\_name) | Name of the Cloud Run service | `string` | `"basemachina-bridge"` | no |
| <a name="input_tenant_id"></a> [tenant\_id](#input\_tenant\_id) | Tenant ID for authentication | `string` | n/a | yes |
| <a name="input_vpc_connector_id"></a> [vpc\_connector\_id](#input\_vpc\_connector\_id) | VPC Access Connector ID for Cloud SQL connection (optional, uses Direct VPC Egress if not specified) | `string` | `null` | no |
| <a name="input_vpc_egress"></a> [vpc\_egress](#input\_vpc\_egress) | VPC egress setting (ALL\_TRAFFIC or PRIVATE\_RANGES\_ONLY) | `string` | `"PRIVATE_RANGES_ONLY"` | no |
| <a name="input_vpc_network_id"></a> [vpc\_network\_id](#input\_vpc\_network\_id) | VPC network ID for Direct VPC Egress (optional, required if using Direct VPC Egress) | `string` | `null` | no |
| <a name="input_vpc_subnetwork_id"></a> [vpc\_subnetwork\_id](#input\_vpc\_subnetwork\_id) | VPC subnetwork ID for Direct VPC Egress (optional, required if using Direct VPC Egress) | `string` | `null` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_backend_service_id"></a> [backend\_service\_id](#output\_backend\_service\_id) | Backend service ID |
| <a name="output_bridge_image_uri"></a> [bridge\_image\_uri](#output\_bridge\_image\_uri) | Bridge container image URI used by Cloud Run service |
| <a name="output_dns_record_fqdn"></a> [dns\_record\_fqdn](#output\_dns\_record\_fqdn) | Fully qualified domain name |
| <a name="output_dns_record_name"></a> [dns\_record\_name](#output\_dns\_record\_name) | DNS record name |
| <a name="output_load_balancer_ip"></a> [load\_balancer\_ip](#output\_load\_balancer\_ip) | Load balancer external IP address |
| <a name="output_service_account_email"></a> [service\_account\_email](#output\_service\_account\_email) | Service account email used by Cloud Run |
| <a name="output_service_id"></a> [service\_id](#output\_service\_id) | Cloud Run service ID |
| <a name="output_service_name"></a> [service\_name](#output\_service\_name) | Cloud Run service name |
| <a name="output_service_url"></a> [service\_url](#output\_service\_url) | Cloud Run service URL |
| <a name="output_ssl_certificate_id"></a> [ssl\_certificate\_id](#output\_ssl\_certificate\_id) | Managed SSL certificate ID |
<!-- END_TF_DOCS -->