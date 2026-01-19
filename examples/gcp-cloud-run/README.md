# BaseMachina Bridge - Google Cloud Run Example

このexampleは、BaseMachina BridgeをGoogle Cloud Run上にデプロイする完全な動作例を提供します。以下のリソースが自動的に作成されます：

- **Cloud Run v2 Service**: Bridgeコンテナを実行
- **VPC Network & Subnet**: プライベートネットワーク
- **Cloud SQL (PostgreSQL)**: プライベートIPでのデータベースインスタンス
- **Cloud Load Balancer**: HTTPS/HTTPトラフィックのルーティング（オプション）
- **Google-managed SSL Certificate**: カスタムドメイン用のSSL証明書（オプション）
- **Cloud Armor**: IPベースのアクセス制御（オプション）
- **Cloud DNS**: ドメイン名のDNSレコード（オプション）

## 前提条件

このexampleを使用する前に、以下を準備してください：

### 1. Google Cloudプロジェクト

有効なGoogle Cloudプロジェクトを用意し、以下のAPIを有効化します：

```bash
# 必須API
gcloud services enable run.googleapis.com
gcloud services enable compute.googleapis.com
gcloud services enable sqladmin.googleapis.com
gcloud services enable servicenetworking.googleapis.com

# オプション（カスタムドメインを使用する場合）
gcloud services enable dns.googleapis.com
```

### 2. 認証情報の設定

Google Cloudへの認証を設定します：

```bash
# サービスアカウントを使用する場合
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/service-account-key.json"

# またはgcloud CLIで認証
gcloud auth application-default login
```

### 3. Terraform

Terraformバージョン1.0以上をインストールしてください：

```bash
terraform version
```

### 4. テナントID

BaseMachinaから提供されるテナントIDを取得してください。

### 5. カスタムドメイン（オプション）

HTTPSでBridgeにアクセスする場合、以下を準備してください：

- カスタムドメイン名（例: `bridge.example.com`）
- Cloud DNS Managed Zone（事前に作成）

Cloud DNS Managed Zoneの作成例：

```bash
gcloud dns managed-zones create example-com \
  --dns-name="example.com." \
  --description="Example domain"
```

## セットアップ手順

### ステップ1: terraform.tfvarsファイルの作成

terraform.tfvars.exampleをコピーして編集します：

```bash
cp terraform.tfvars.example terraform.tfvars
```

`terraform.tfvars`を編集して、以下の必須項目を設定してください：

```hcl
project_id = "your-gcp-project-id"
tenant_id  = "your-tenant-id"
```

カスタムドメインを使用する場合は、以下も設定します：

```hcl
domain_name   = "bridge.example.com"
dns_zone_name = "example-com"  # Cloud DNS Managed Zone名
```

### ステップ2: Terraformの初期化

```bash
terraform init
```

### ステップ3: 実行プランの確認

```bash
terraform plan
```

以下のリソースが作成されることを確認してください：
- VPCネットワークとサブネット
- Cloud SQLインスタンス（プライベートIP）
- Cloud Runサービス
- Load Balancer、SSL証明書、Cloud Armor（domain_nameを設定した場合）

### ステップ4: インフラストラクチャのデプロイ

```bash
terraform apply
```

プロンプトで `yes` と入力してデプロイを開始します。

**注**: Cloud SQLインスタンスとSSL証明書のプロビジョニングには10〜15分程度かかる場合があります。

### ステップ5: デプロイ結果の確認

デプロイが完了すると、以下の情報が出力されます：

```bash
# Bridgeサービス情報
bridge_service_url          = "https://basemachina-bridge-example-xxxxx-an.a.run.app"
bridge_domain_url           = "https://bridge.example.com"
bridge_load_balancer_ip     = "203.0.113.10"

# Cloud SQL情報
cloud_sql_connection_name   = "your-project:asia-northeast1:instance-name"
cloud_sql_private_ip        = "10.x.x.x"
database_name               = "sampledb"
database_user               = "dbuser"
```

データベースパスワードを確認するには：

```bash
terraform output -raw database_password
```

## テスト方法

### 1. HTTPS疎通確認

カスタムドメインを設定した場合、HTTPSでBridgeにアクセスできることを確認します：

```bash
curl -v https://bridge.example.com/ok
```

期待される応答:
```
HTTP/2 200
content-type: text/plain
...
OK
```

### 2. Cloud Run直接アクセス

Cloud RunサービスのURLに直接アクセスすることもできます（内部ロードバランサー経由のみアクセス可能）：

```bash
# サービスURLを取得
SERVICE_URL=$(terraform output -raw bridge_service_url)

# Cloud Runサービスにリクエスト（認証が必要）
curl -H "Authorization: Bearer $(gcloud auth print-identity-token)" \
  "${SERVICE_URL}/ok"
```

### 3. Cloud SQL接続テスト

Cloud SQLインスタンスに接続してデータベースを初期化します。

#### オプション1: Cloud SQL Proxyを使用

```bash
# Cloud SQL Proxyのインストール（macOS）
brew install cloud-sql-proxy

# Cloud SQL接続名を取得
CONNECTION_NAME=$(terraform output -raw cloud_sql_connection_name)

# Cloud SQL Proxyを起動
cloud-sql-proxy --port 5432 "${CONNECTION_NAME}"

# 別のターミナルでpsqlを使用して接続
DB_PASSWORD=$(terraform output -raw database_password)
psql "host=127.0.0.1 port=5432 dbname=sampledb user=dbuser password=${DB_PASSWORD}"
```

#### オプション2: VPCネットワーク経由で接続

VPCネットワーク内のCompute Engineインスタンスから接続する場合：

```bash
# Cloud SQLのプライベートIPを取得
DB_IP=$(terraform output -raw cloud_sql_private_ip)

# psqlで接続
psql "host=${DB_IP} port=5432 dbname=sampledb user=dbuser password=${DB_PASSWORD}"
```

### 4. データベース初期化

接続が確認できたら、サンプルデータを投入します：

```bash
psql "host=127.0.0.1 port=5432 dbname=sampledb user=dbuser password=${DB_PASSWORD}" \
  < scripts/init.sql
```

以下のテーブルとサンプルデータが作成されます：
- `users` テーブル（3件）
- `products` テーブル（3件）
- `orders` テーブル（3件）

### 5. Bridge経由でのデータアクセステスト

Bridgeを経由してCloud SQLのデータにアクセスできることを確認します（BaseMachinaプラットフォーム経由）。

## クリーンアップ

すべてのリソースを削除するには：

```bash
terraform destroy
```

プロンプトで `yes` と入力してリソースを削除します。

**注意**: 以下のリソースは手動で削除が必要な場合があります：
- Cloud DNS Managed Zone（既存のものを使用した場合）
- VPCピアリング接続（削除に時間がかかる場合があります）

## トラブルシューティング

### SSL証明書のプロビジョニングが完了しない

**症状**: `terraform apply`がSSL証明書の作成で長時間待機する

**解決方法**:
1. DNSレコードが正しく設定されているか確認:
   ```bash
   dig +short bridge.example.com
   ```
2. Load BalancerのIPアドレスが返されることを確認
3. SSL証明書のプロビジョニングには最大15分かかる場合があります

### VPC Peering接続エラー

**症状**: `Error creating service networking connection`

**解決方法**:
1. Service Networking APIが有効化されているか確認:
   ```bash
   gcloud services enable servicenetworking.googleapis.com
   ```
2. VPCネットワークにプライベートサービス接続用のIPアドレス範囲が確保されているか確認

### Cloud SQLインスタンスに接続できない

**症状**: `FATAL: password authentication failed`

**解決方法**:
1. データベースパスワードが正しいか確認:
   ```bash
   terraform output -raw database_password
   ```
2. Cloud SQLインスタンスがプライベートIPのみで構成されているため、VPCネットワーク経由またはCloud SQL Proxy経由でアクセスする必要があります

### Cloud Armorによるアクセス拒否

**症状**: `HTTP 403 Forbidden`

**解決方法**:
1. `allowed_ip_ranges`に自分のIPアドレスが含まれているか確認
2. デフォルトではBaseMachinaのIP（34.85.43.93/32）のみが許可されています
3. テスト時は自分のIPアドレスを追加してください:
   ```hcl
   allowed_ip_ranges = ["34.85.43.93/32", "YOUR_IP/32"]
   ```

### Terraformステートファイルのロック

**症状**: `Error acquiring the state lock`

**解決方法**:
1. 別のTerraform操作が実行中でないか確認
2. ロックが残っている場合は手動で解除:
   ```bash
   terraform force-unlock LOCK_ID
   ```

## 追加設定

### リソース設定のカスタマイズ

Cloud Runサービスのリソースを調整する場合、`terraform.tfvars`で以下を設定します：

```hcl
cpu           = "2"      # CPU数
memory        = "1Gi"    # メモリサイズ
min_instances = 1        # 最小インスタンス数
max_instances = 20       # 最大インスタンス数
```

### 複数のIPアドレスを許可

複数のIPアドレスからのアクセスを許可する場合：

```hcl
allowed_ip_ranges = [
  "34.85.43.93/32",      # BaseMachina
  "203.0.113.0/24",      # Office network
  "198.51.100.10/32"     # VPN IP
]
```

### ラベルのカスタマイズ

すべてのリソースに適用されるラベルをカスタマイズできます：

```hcl
labels = {
  environment = "production"
  team        = "data-platform"
  cost_center = "engineering"
}
```

## 参考資料

- [Cloud Run Documentation](https://cloud.google.com/run/docs)
- [Cloud SQL Documentation](https://cloud.google.com/sql/docs)
- [Cloud Load Balancing Documentation](https://cloud.google.com/load-balancing/docs)
- [Cloud Armor Documentation](https://cloud.google.com/armor/docs)
- [BaseMachina Bridge Module README](../../modules/gcp/cloud-run/README.md)

<!-- BEGIN_TF_DOCS -->
## Requirements

| Name | Version |
|------|---------|
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.0 |
| <a name="requirement_google"></a> [google](#requirement\_google) | ~> 5.0 |
| <a name="requirement_null"></a> [null](#requirement\_null) | ~> 3.0 |
| <a name="requirement_random"></a> [random](#requirement\_random) | ~> 3.0 |

## Providers

| Name | Version |
|------|---------|
| <a name="provider_google"></a> [google](#provider\_google) | 5.45.2 |
| <a name="provider_null"></a> [null](#provider\_null) | 3.2.4 |
| <a name="provider_random"></a> [random](#provider\_random) | 3.7.2 |

## Modules

| Name | Source | Version |
|------|--------|---------|
| <a name="module_basemachina_bridge"></a> [basemachina\_bridge](#module\_basemachina\_bridge) | ../../modules/gcp/cloud-run | n/a |

## Resources

| Name | Type |
|------|------|
| [google_compute_global_address.private_ip_address](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_global_address) | resource |
| [google_compute_network.main](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_network) | resource |
| [google_compute_subnetwork.main](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/compute_subnetwork) | resource |
| [google_service_networking_connection.private_vpc_connection](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/service_networking_connection) | resource |
| [google_sql_database.database](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/sql_database) | resource |
| [google_sql_database_instance.main](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/sql_database_instance) | resource |
| [google_sql_user.user](https://registry.terraform.io/providers/hashicorp/google/latest/docs/resources/sql_user) | resource |
| [null_resource.cleanup_cloud_sql](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |
| [null_resource.cleanup_vpc_peering](https://registry.terraform.io/providers/hashicorp/null/latest/docs/resources/resource) | resource |
| [random_id.db_name_suffix](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/id) | resource |
| [random_password.db_password](https://registry.terraform.io/providers/hashicorp/random/latest/docs/resources/password) | resource |
| [google_dns_managed_zone.main](https://registry.terraform.io/providers/hashicorp/google/latest/docs/data-sources/dns_managed_zone) | data source |

## Inputs

| Name | Description | Type | Default | Required |
|------|-------------|------|---------|:--------:|
| <a name="input_allowed_ip_ranges"></a> [allowed\_ip\_ranges](#input\_allowed\_ip\_ranges) | IP ranges allowed to access the service | `list(string)` | <pre>[<br/>  "34.85.43.93/32"<br/>]</pre> | no |
| <a name="input_cpu"></a> [cpu](#input\_cpu) | CPU allocation for Cloud Run service | `string` | `"1"` | no |
| <a name="input_database_name"></a> [database\_name](#input\_database\_name) | Database name | `string` | `"sampledb"` | no |
| <a name="input_database_user"></a> [database\_user](#input\_database\_user) | Database user name | `string` | `"dbuser"` | no |
| <a name="input_dns_zone_name"></a> [dns\_zone\_name](#input\_dns\_zone\_name) | Cloud DNS Managed Zone name (optional) | `string` | `null` | no |
| <a name="input_domain_name"></a> [domain\_name](#input\_domain\_name) | Custom domain name for the Bridge (optional) | `string` | `null` | no |
| <a name="input_enable_cloud_armor"></a> [enable\_cloud\_armor](#input\_enable\_cloud\_armor) | Enable Cloud Armor security policy | `bool` | `true` | no |
| <a name="input_enable_https_redirect"></a> [enable\_https\_redirect](#input\_enable\_https\_redirect) | Enable HTTP to HTTPS redirect | `bool` | `true` | no |
| <a name="input_fetch_interval"></a> [fetch\_interval](#input\_fetch\_interval) | Interval for fetching public keys | `string` | `"1h"` | no |
| <a name="input_fetch_timeout"></a> [fetch\_timeout](#input\_fetch\_timeout) | Timeout for fetching public keys | `string` | `"10s"` | no |
| <a name="input_labels"></a> [labels](#input\_labels) | Labels to apply to all resources | `map(string)` | <pre>{<br/>  "environment": "example",<br/>  "managed_by": "terraform"<br/>}</pre> | no |
| <a name="input_max_instances"></a> [max\_instances](#input\_max\_instances) | Maximum number of instances | `number` | `10` | no |
| <a name="input_memory"></a> [memory](#input\_memory) | Memory allocation for Cloud Run service | `string` | `"512Mi"` | no |
| <a name="input_min_instances"></a> [min\_instances](#input\_min\_instances) | Minimum number of instances | `number` | `0` | no |
| <a name="input_port"></a> [port](#input\_port) | Port number for Bridge container | `number` | `8080` | no |
| <a name="input_project_id"></a> [project\_id](#input\_project\_id) | Google Cloud Project ID | `string` | n/a | yes |
| <a name="input_region"></a> [region](#input\_region) | Google Cloud region | `string` | `"asia-northeast1"` | no |
| <a name="input_service_name"></a> [service\_name](#input\_service\_name) | Service name prefix for all resources | `string` | `"basemachina-bridge-example"` | no |
| <a name="input_tenant_id"></a> [tenant\_id](#input\_tenant\_id) | Tenant ID for BaseMachina Bridge | `string` | n/a | yes |
| <a name="input_vpc_egress"></a> [vpc\_egress](#input\_vpc\_egress) | VPC egress setting | `string` | `"PRIVATE_RANGES_ONLY"` | no |

## Outputs

| Name | Description |
|------|-------------|
| <a name="output_bridge_domain_url"></a> [bridge\_domain\_url](#output\_bridge\_domain\_url) | Bridge domain URL (if domain\_name is configured) |
| <a name="output_bridge_load_balancer_ip"></a> [bridge\_load\_balancer\_ip](#output\_bridge\_load\_balancer\_ip) | Load Balancer external IP address |
| <a name="output_bridge_service_account_email"></a> [bridge\_service\_account\_email](#output\_bridge\_service\_account\_email) | Service account email used by Cloud Run |
| <a name="output_bridge_service_name"></a> [bridge\_service\_name](#output\_bridge\_service\_name) | Cloud Run service name |
| <a name="output_bridge_service_url"></a> [bridge\_service\_url](#output\_bridge\_service\_url) | Cloud Run service URL |
| <a name="output_cloud_sql_connection_name"></a> [cloud\_sql\_connection\_name](#output\_cloud\_sql\_connection\_name) | Cloud SQL connection name |
| <a name="output_cloud_sql_instance_name"></a> [cloud\_sql\_instance\_name](#output\_cloud\_sql\_instance\_name) | Cloud SQL instance name |
| <a name="output_cloud_sql_private_ip"></a> [cloud\_sql\_private\_ip](#output\_cloud\_sql\_private\_ip) | Cloud SQL private IP address |
| <a name="output_database_name"></a> [database\_name](#output\_database\_name) | Database name |
| <a name="output_database_password"></a> [database\_password](#output\_database\_password) | Database password (sensitive) |
| <a name="output_database_user"></a> [database\_user](#output\_database\_user) | Database user name |
| <a name="output_dns_record_fqdn"></a> [dns\_record\_fqdn](#output\_dns\_record\_fqdn) | Fully qualified domain name |
| <a name="output_dns_zone_name"></a> [dns\_zone\_name](#output\_dns\_zone\_name) | DNS Managed Zone name |
| <a name="output_subnet_id"></a> [subnet\_id](#output\_subnet\_id) | Subnet ID |
| <a name="output_vpc_network_id"></a> [vpc\_network\_id](#output\_vpc\_network\_id) | VPC network ID |
| <a name="output_vpc_network_name"></a> [vpc\_network\_name](#output\_vpc\_network\_name) | VPC network name |
<!-- END_TF_DOCS -->