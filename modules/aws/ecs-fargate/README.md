# BaseMachina Bridge - AWS ECS Fargate Module

## モジュール概要

このTerraformモジュールは、BaseMachina Bridgeを AWS ECS Fargate 環境に自動的にデプロイします。BridgeはBaseMachinaからお客様のプライベートデータソース（RDS、内部API等）への安全なアクセスを実現する認証機能付きゲートウェイです。

### 主な機能

- **サーバーレスコンテナ実行**: ECS Fargateによるインフラ管理不要の運用
- **セキュアなネットワーク**: プライベートサブネット配置、IPホワイトリスト、HTTPS通信
- **自動ヘルスチェック**: ALBによる健全性監視とタスク自動復旧
- **ログ集約**: CloudWatch Logsによる一元的なログ管理
- **柔軟な設定**: 変数による環境変数、リソースサイズ、タグのカスタマイズ

## 使用方法

### 基本的な使用例

```hcl
module "bridge" {
  source = "../../modules/aws/ecs-fargate"

  # ネットワーク設定
  vpc_id             = "vpc-xxxxx"
  private_subnet_ids = ["subnet-xxxxx", "subnet-yyyyy"]
  public_subnet_ids  = ["subnet-aaaaa", "subnet-bbbbb"]

  # SSL/TLS証明書
  certificate_arn = "arn:aws:acm:ap-northeast-1:123456789012:certificate/xxxxx"

  # Bridge環境変数
  tenant_id      = "your-tenant-id"
  fetch_interval = "1h"
  fetch_timeout  = "10s"
  port           = 8080

  # リソース設定
  cpu            = 256
  memory         = 512
  desired_count  = 1
  # bridge_image_tag = "v1.0.0"  # オプション: 特定バージョンを指定（デフォルト: latest）

  # カスタムドメイン設定（必須）
  domain_name      = "bridge.example.com"
  route53_zone_id  = "Z1234567890ABC"  # example.com のHosted Zone ID

  # タグ
  tags = {
    Environment = "production"
    Project     = "basemachina-bridge"
  }

  name_prefix = "prod"
}
```

## 要件

### Terraformバージョン

- Terraform: >= 1.5

### プロバイダー

- AWS Provider: ~> 6.0

### 前提条件

デプロイ前に、以下のリソースが既に存在している必要があります：

1. **VPC**: 既存のVPCとサブネット（パブリック・プライベート）
2. **ACM証明書**: HTTPS通信用のSSL/TLS証明書
3. **AWS認証情報**: Terraformを実行するためのIAM権限

**注**: NAT Gatewayは自動的に作成されます（Public ECRアクセスとBaseMachina認証サーバー接続用、既存のNAT Gateway IDを指定することも可能）。VPCエンドポイント（ECR API/DKR、S3、CloudWatch Logs）とECRプルスルーキャッシュもデフォルトで有効化されます。

### ネットワーク構成の詳細

このモジュールは、セキュリティとコスト効率を両立するために、VPCエンドポイントとNAT Gatewayのハイブリッド構成を採用しています。

#### NAT Gateway（必須）

プライベートサブネット内のBridgeタスクがインターネットにアクセスするために必要です。

- **役割**: Public ECR（`public.ecr.aws`）からBridgeコンテナイメージをプルするための外部アクセスを提供
- **作成**: デフォルトで新規作成、または既存のNAT Gateway IDを`nat_gateway_id`変数で指定可能
- **配置**: パブリックサブネットに配置され、Elastic IPが割り当てられる

#### VPCエンドポイント（推奨）

プライベートサブネット内のリソースがインターネットを経由せずにAWSサービスにアクセスできます。

- **ECR API/DKR エンドポイント**: Private ECRへのアクセスに使用（将来の拡張用）
- **S3 エンドポイント**: ECRレイヤーの取得に使用（ゲートウェイ型、追加コストなし）
- **CloudWatch Logs エンドポイント**: ログ送信に使用
- **利点**: データ転送コスト削減、レイテンシ低減、セキュリティ向上

#### ECRプルスルーキャッシュ

Public ECRのイメージをPrivate ECRにキャッシュする機能です。

- **仕組み**: `public.ecr.aws/basemachina/bridge`のイメージを自動的にPrivate ECRにキャッシュ
- **利点**:
  - Public ECRのレート制限回避
  - イメージプル速度の向上
  - 可用性の向上（Public ECR障害時も動作）
- **設定**: `aws_ecr_pull_through_cache_rule.public_ecr`リソースで自動作成

#### 推奨構成

**VPCエンドポイント + NAT Gateway**のハイブリッド構成を推奨します：

- VPCエンドポイント: Private ECR、S3、CloudWatch Logsへの効率的なアクセス
- NAT Gateway: Public ECRへのアクセス（初回イメージプル時）
- ECRプルスルーキャッシュ: Public → Private ECRキャッシュによる安定性向上

この構成により、セキュリティ、コスト効率、可用性のバランスが最適化されます。

詳細な前提条件については、[examples/aws-ecs-fargate/README.md](../../examples/aws-ecs-fargate/README.md) を参照してください。

## 入力変数

<!-- BEGIN_TF_DOCS -->


## 要件

## Requirements

| Name | Version |
| ---- | ------- |
| <a name="requirement_terraform"></a> [terraform](#requirement\_terraform) | >= 1.5 |
| <a name="requirement_aws"></a> [aws](#requirement\_aws) | ~> 6.0 |

## プロバイダー

## Providers

| Name | Version |
| ---- | ------- |
| <a name="provider_aws"></a> [aws](#provider\_aws) | 6.54.0 |

## モジュール

## Modules

No modules.

## リソース

## Resources

| Name | Type |
| ---- | ---- |
| [aws_cloudwatch_log_group.bridge](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/cloudwatch_log_group) | resource |
| [aws_ecr_pull_through_cache_rule.public_ecr](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/ecr_pull_through_cache_rule) | resource |
| [aws_ecs_cluster.main](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/ecs_cluster) | resource |
| [aws_ecs_service.bridge](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/ecs_service) | resource |
| [aws_ecs_task_definition.bridge](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/ecs_task_definition) | resource |
| [aws_eip.nat](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/eip) | resource |
| [aws_iam_role.task_execution](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_role) | resource |
| [aws_iam_role_policy.cloudwatch_logs](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_role_policy) | resource |
| [aws_iam_role_policy_attachment.ecr_read_only](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_role_policy_attachment) | resource |
| [aws_iam_role_policy_attachment.task_execution](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/iam_role_policy_attachment) | resource |
| [aws_lb.main](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lb) | resource |
| [aws_lb_listener.https](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lb_listener) | resource |
| [aws_lb_target_group.bridge](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/lb_target_group) | resource |
| [aws_nat_gateway.bridge](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/nat_gateway) | resource |
| [aws_route.private_nat_gateway](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/route) | resource |
| [aws_route53_record.bridge](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/route53_record) | resource |
| [aws_security_group.alb](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/security_group) | resource |
| [aws_security_group.bridge](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/security_group) | resource |
| [aws_security_group.vpc_endpoints](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/security_group) | resource |
| [aws_security_group_rule.alb_egress_all](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/security_group_rule) | resource |
| [aws_security_group_rule.alb_ingress_https_additional](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/security_group_rule) | resource |
| [aws_security_group_rule.alb_ingress_https_basemachina](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/security_group_rule) | resource |
| [aws_security_group_rule.bridge_egress_all](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/security_group_rule) | resource |
| [aws_security_group_rule.bridge_ingress_http](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/security_group_rule) | resource |
| [aws_security_group_rule.vpc_endpoints_egress_all](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/security_group_rule) | resource |
| [aws_security_group_rule.vpc_endpoints_ingress_https](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/security_group_rule) | resource |
| [aws_vpc_endpoint.ecr_api](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/vpc_endpoint) | resource |
| [aws_vpc_endpoint.ecr_dkr](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/vpc_endpoint) | resource |
| [aws_vpc_endpoint.logs](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/vpc_endpoint) | resource |
| [aws_vpc_endpoint.s3](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/vpc_endpoint) | resource |
| [aws_caller_identity.current](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/caller_identity) | data source |
| [aws_region.current](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/region) | data source |
| [aws_route_table.private_subnet](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/data-sources/route_table) | data source |

## 入力変数

## Inputs

| Name | Description | Type | Default | Required |
| ---- | ----------- | ---- | ------- | :------: |
| <a name="input_additional_alb_ingress_cidrs"></a> [additional\_alb\_ingress\_cidrs](#input\_additional\_alb\_ingress\_cidrs) | Additional CIDR blocks to allow HTTPS access to ALB (for testing or additional clients). BaseMachina IP (34.85.43.93/32) is always included. | `list(string)` | `[]` | no |
| <a name="input_bridge_image_tag"></a> [bridge\_image\_tag](#input\_bridge\_image\_tag) | Bridge container image tag (default: latest). Specify a specific version like 'v1.0.0' if needed. | `string` | `"latest"` | no |
| <a name="input_certificate_arn"></a> [certificate\_arn](#input\_certificate\_arn) | ACM certificate ARN for HTTPS listener (required) | `string` | n/a | yes |
| <a name="input_cpu"></a> [cpu](#input\_cpu) | CPU units for ECS task (256, 512, 1024, 2048, 4096) | `number` | `256` | no |
| <a name="input_create_nat_gateway"></a> [create\_nat\_gateway](#input\_create\_nat\_gateway) | Whether to manage NAT Gateway resources (NAT Gateway, EIP and default routes for the private subnets). Set to false when the VPC already has NAT routing configured for the private subnets. When true with nat\_gateway\_id specified, the NAT Gateway itself is not created and only the default routes to the existing NAT Gateway are added. | `bool` | `true` | no |
| <a name="input_create_vpc_endpoints"></a> [create\_vpc\_endpoints](#input\_create\_vpc\_endpoints) | Whether to create VPC endpoints (ECR API/DKR, S3, CloudWatch Logs). Set to false when the VPC already has these endpoints, because only one interface endpoint with private DNS enabled can exist per service in a VPC. | `bool` | `true` | no |
| <a name="input_desired_count"></a> [desired\_count](#input\_desired\_count) | Number of ECS tasks to run | `number` | `1` | no |
| <a name="input_domain_name"></a> [domain\_name](#input\_domain\_name) | Custom domain name for the Bridge (required). This domain will be used for ALB access. An A record alias to ALB will be created automatically in the specified Route53 Hosted Zone. | `string` | n/a | yes |
| <a name="input_enable_ecr_pull_through_cache"></a> [enable\_ecr\_pull\_through\_cache](#input\_enable\_ecr\_pull\_through\_cache) | Whether to create an ECR pull through cache rule and pull the Bridge image through the private ECR registry. Set to false to pull the image directly from public.ecr.aws (requires internet access via NAT Gateway). | `bool` | `true` | no |
| <a name="input_fetch_interval"></a> [fetch\_interval](#input\_fetch\_interval) | Interval for fetching public keys (e.g., 1h, 30m) | `string` | `"1h"` | no |
| <a name="input_fetch_timeout"></a> [fetch\_timeout](#input\_fetch\_timeout) | Timeout for fetching public keys (e.g., 10s, 30s) | `string` | `"10s"` | no |
| <a name="input_log_retention_days"></a> [log\_retention\_days](#input\_log\_retention\_days) | CloudWatch Logs retention period (days) | `number` | `7` | no |
| <a name="input_memory"></a> [memory](#input\_memory) | Memory (MiB) for ECS task | `number` | `512` | no |
| <a name="input_name_prefix"></a> [name\_prefix](#input\_name\_prefix) | Prefix for resource names. Up to 8 characters, alphanumeric and hyphens only, and must not start or end with a hyphen. | `string` | `""` | no |
| <a name="input_nat_gateway_id"></a> [nat\_gateway\_id](#input\_nat\_gateway\_id) | Existing NAT Gateway ID to use (optional). If not specified, a new NAT Gateway will be created for Bridge. | `string` | `null` | no |
| <a name="input_port"></a> [port](#input\_port) | Port number for Bridge container (cannot be 4321) | `number` | `8080` | no |
| <a name="input_private_subnet_ids"></a> [private\_subnet\_ids](#input\_private\_subnet\_ids) | List of private subnet IDs for ECS tasks | `list(string)` | n/a | yes |
| <a name="input_public_subnet_ids"></a> [public\_subnet\_ids](#input\_public\_subnet\_ids) | List of public subnet IDs for ALB and NAT Gateway (if creating new NAT Gateway) | `list(string)` | n/a | yes |
| <a name="input_route53_zone_id"></a> [route53\_zone\_id](#input\_route53\_zone\_id) | Route53 Hosted Zone ID for DNS record creation (required). An A record alias pointing to the ALB will be created automatically in this zone. | `string` | n/a | yes |
| <a name="input_ssl_policy"></a> [ssl\_policy](#input\_ssl\_policy) | SSL security policy for the ALB HTTPS listener | `string` | `"ELBSecurityPolicy-TLS13-1-2-Res-PQ-2025-09"` | no |
| <a name="input_tags"></a> [tags](#input\_tags) | Common tags to apply to all resources | `map(string)` | `{}` | no |
| <a name="input_tenant_id"></a> [tenant\_id](#input\_tenant\_id) | Tenant ID for authentication | `string` | n/a | yes |
| <a name="input_vpc_id"></a> [vpc\_id](#input\_vpc\_id) | VPC ID where the resources will be created | `string` | n/a | yes |

## 出力値

## Outputs

| Name | Description |
| ---- | ----------- |
| <a name="output_alb_arn"></a> [alb\_arn](#output\_alb\_arn) | ALBのARN（リソース参照用） |
| <a name="output_alb_dns_name"></a> [alb\_dns\_name](#output\_alb\_dns\_name) | ALBのDNS名（Route 53レコード作成用） |
| <a name="output_alb_security_group_id"></a> [alb\_security\_group\_id](#output\_alb\_security\_group\_id) | ALBセキュリティグループのID（通信ルール設定用） |
| <a name="output_bridge_image_uri"></a> [bridge\_image\_uri](#output\_bridge\_image\_uri) | 使用されているBridgeコンテナイメージURI |
| <a name="output_bridge_security_group_id"></a> [bridge\_security\_group\_id](#output\_bridge\_security\_group\_id) | BridgeセキュリティグループのID（他リソースとの通信ルール設定用） |
| <a name="output_cloudwatch_log_group_name"></a> [cloudwatch\_log\_group\_name](#output\_cloudwatch\_log\_group\_name) | CloudWatch Logsロググループ名（ログ確認用） |
| <a name="output_domain_name"></a> [domain\_name](#output\_domain\_name) | 設定されたカスタムドメイン名 |
| <a name="output_ecs_cluster_arn"></a> [ecs\_cluster\_arn](#output\_ecs\_cluster\_arn) | ECSクラスターARN（リソース参照用） |
| <a name="output_ecs_cluster_name"></a> [ecs\_cluster\_name](#output\_ecs\_cluster\_name) | ECSクラスター名（AWS CLIやモニタリング用） |
| <a name="output_ecs_service_name"></a> [ecs\_service\_name](#output\_ecs\_service\_name) | ECSサービス名（デプロイやスケーリング用） |
| <a name="output_nat_gateway_id"></a> [nat\_gateway\_id](#output\_nat\_gateway\_id) | NAT Gateway ID (created or existing) |
| <a name="output_nat_gateway_public_ip"></a> [nat\_gateway\_public\_ip](#output\_nat\_gateway\_public\_ip) | NAT Gateway public IP address (null if using existing NAT Gateway) |
| <a name="output_route53_record_fqdn"></a> [route53\_record\_fqdn](#output\_route53\_record\_fqdn) | Route53レコードのFQDN |
| <a name="output_route53_zone_id"></a> [route53\_zone\_id](#output\_route53\_zone\_id) | 使用されたRoute53 Hosted Zone ID |
| <a name="output_task_execution_role_arn"></a> [task\_execution\_role\_arn](#output\_task\_execution\_role\_arn) | タスク実行ロールARN（権限管理用） |
| <a name="output_vpc_endpoint_ecr_api_id"></a> [vpc\_endpoint\_ecr\_api\_id](#output\_vpc\_endpoint\_ecr\_api\_id) | ECR API VPCエンドポイントID（create\_vpc\_endpoints = falseの場合はnull） |
| <a name="output_vpc_endpoint_ecr_dkr_id"></a> [vpc\_endpoint\_ecr\_dkr\_id](#output\_vpc\_endpoint\_ecr\_dkr\_id) | ECR Docker VPCエンドポイントID（create\_vpc\_endpoints = falseの場合はnull） |
| <a name="output_vpc_endpoint_logs_id"></a> [vpc\_endpoint\_logs\_id](#output\_vpc\_endpoint\_logs\_id) | CloudWatch Logs VPCエンドポイントID（create\_vpc\_endpoints = falseの場合はnull） |
| <a name="output_vpc_endpoint_s3_id"></a> [vpc\_endpoint\_s3\_id](#output\_vpc\_endpoint\_s3\_id) | S3 VPCエンドポイントID（create\_vpc\_endpoints = falseの場合はnull） |
| <a name="output_vpc_endpoints_security_group_id"></a> [vpc\_endpoints\_security\_group\_id](#output\_vpc\_endpoints\_security\_group\_id) | VPCエンドポイント用セキュリティグループID（create\_vpc\_endpoints = falseの場合はnull） |
<!-- END_TF_DOCS -->

## 例

実際の使用例は [examples/aws-ecs-fargate/](../../examples/aws-ecs-fargate/) ディレクトリを参照してください。

### カスタムドメインとRoute53の設定

Bridgeにカスタムドメインでアクセスしたい場合、Route53のDNSレコードを自動作成できます:

```hcl
module "bridge" {
  source = "../../modules/aws/ecs-fargate"

  # 基本設定...
  vpc_id             = "vpc-xxxxx"
  private_subnet_ids = ["subnet-xxxxx", "subnet-yyyyy"]
  public_subnet_ids  = ["subnet-aaaaa", "subnet-bbbbb"]
  certificate_arn    = "arn:aws:acm:ap-northeast-1:123456789012:certificate/xxxxx"
  tenant_id          = "your-tenant-id"

  # カスタムドメイン設定
  domain_name      = "bridge.example.com"
  route53_zone_id  = "Z1234567890ABC"  # example.com のHosted Zone ID

  name_prefix = "prod"
}

# 作成されたFQDNを出力
output "bridge_url" {
  value = "https://${module.bridge.route53_record_fqdn}"
}
```

**注**:
- `domain_name`と`route53_zone_id`は必須パラメータです
- Route53 Hosted Zoneに自動的にAレコード（ALBへのエイリアス）が作成されます
- ACM証明書はドメイン名をカバーしている必要があります
- Route53 Hosted Zoneは事前に作成されている必要があります

## 証明書オプション

このモジュールはHTTPS通信を必須とするため、`certificate_arn`変数でACM証明書のARNを指定する必要があります。以下のいずれかの方法で証明書を準備できます：

### 1. DNS検証によるACM証明書自動発行（推奨）

Route53でDNS検証を使用してACM証明書を自動的に発行します。Route53 Hosted Zoneが必要です。

- **利点**: 自動更新、AWSマネージド、最も簡単
- **要件**: Route53 Hosted Zone、ドメイン所有権
- **設定**: [examples/aws-ecs-fargate/](../../examples/aws-ecs-fargate/) の `acm.tf` を参照

### 2. 自己署名証明書のACMインポート

テスト環境向けに自己署名証明書を生成してACMにインポートします。

- **利点**: 外部ドメイン不要、テスト環境に最適
- **欠点**: ブラウザ警告、手動更新必要
- **設定**: [examples/aws-ecs-fargate/scripts/generate-cert.sh](../../examples/aws-ecs-fargate/scripts/generate-cert.sh) を使用

### 3. 既存のACM証明書の利用

既にAWS ACMに登録されている証明書を使用します。

- **利点**: 既存のインフラと統合
- **要件**: 証明書がドメイン名をカバーしていること
- **設定**: `certificate_arn`変数に証明書ARNを指定

**注**: HTTPのみの構成はサポートされていません。`certificate_arn`は必須パラメータです。

詳細な設定方法と各オプションの使用例については、[examples/aws-ecs-fargate/README.md](../../examples/aws-ecs-fargate/README.md) を参照してください。

## セキュリティベストプラクティス

### プライベートサブネット配置

Bridgeタスクは必ずプライベートサブネットに配置してください。これにより、インターネットからの直接アクセスを防止し、攻撃面を最小化します。

### IPホワイトリスト

ALBのセキュリティグループは、デフォルトでBaseMachinaのIPアドレス（34.85.43.93/32）からのアクセスのみを許可します。テスト環境では`additional_alb_ingress_cidrs`変数を使用して追加のCIDRブロックを許可できます。本番環境ではこの設定を最小限に保ってください。

### 機密情報の管理

テナントIDやその他の機密情報は、以下の方法で安全に管理してください：

- **AWS Secrets Manager**: 推奨される方法
- **Systems Manager Parameter Store**: 代替方法
- **Terraform変数の暗号化**: terraform.tfvarsファイルをGitにコミットしない

### HTTPS通信の強制

ALBはHTTPS（ポート443）のみを受け付けます。ACM証明書を使用してTLS 1.2以上で通信を暗号化します。

### CloudWatch Logsの監視

Bridgeコンテナのログは CloudWatch Logs（`/ecs/basemachina-bridge`）に集約されます。定期的にログを確認し、以下の監視を推奨します：

- エラーログのフィルタリングとアラート設定
- 認証失敗（401エラー）の監視
- ALBヘルスチェック失敗の検知

## ライセンス

このモジュールはBaseMachinaプロジェクトの一部です。
