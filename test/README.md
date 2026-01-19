# Terratest Integration Tests

このディレクトリには、BaseMachina Terraform モジュールの統合テストが含まれています。

## 前提条件

### 1. Go 1.21以上

```bash
go version
```

### 2. AWS認証情報

テストを実行するには、AWS認証情報が必要です。以下のいずれかの方法で設定してください：

#### 環境変数

```bash
cp .env.example .env
# .envを編集して実際の値を設定
```

必須環境変数：
- `AWS_ACCESS_KEY_ID`: AWSアクセスキー
- `AWS_SECRET_ACCESS_KEY`: AWSシークレットキー
- `AWS_DEFAULT_REGION`: AWSリージョン（例: `ap-northeast-1`）
- `TEST_VPC_ID`: テスト用VPC ID
- `TEST_PRIVATE_SUBNET_IDS`: プライベートサブネットIDのカンマ区切りリスト（例: `subnet-xxx,subnet-yyy`）
- `TEST_PUBLIC_SUBNET_IDS`: パブリックサブネットIDのカンマ区切りリスト（例: `subnet-aaa,subnet-bbb`）
- `TEST_TENANT_ID`: BaseMachinaテナントID
- `TEST_BRIDGE_DOMAIN_NAME`: BridgeのFQDN（例: `bridge-test.example.com`）
- `TEST_ROUTE53_ZONE_ID`: Route53 Hosted Zone ID（例: `Z1234567890ABC`）
  - ACM証明書がDNS検証で自動発行されます
  - Route53にAレコードが自動作成されます

**ネットワーク構成**:

モジュールは**VPCエンドポイント + NAT Gateway のハイブリッド構成**を自動的にデプロイします：

- **VPCエンドポイント**（自動作成）: Private ECR API/DKR、S3、CloudWatch Logs用
  - コスト削減とセキュリティ向上
  - プライベートサブネットからAWSサービスへの効率的なアクセス

- **NAT Gateway**（必須、自動作成）:
  - Public ECR (public.ecr.aws) からのBridgeイメージプル用
  - BaseMachina認証サーバーへのアクセス用
  - **注**: Public ECRはVPCエンドポイントをサポートしていないため、NAT Gatewayが必須です

- **ECRプルスルーキャッシュ**（自動作成）:
  - Public ECRイメージをPrivate ECRにキャッシュ
  - 初回プル後はVPCエンドポイント経由でアクセス可能

オプション環境変数：
- `TEST_DESIRED_COUNT`: デプロイするECSタスク数（デフォルト: 1）

**注**: 以下のRDS関連環境変数はTerratestでは不要です（Bridge単体テストのため）：
- `TEST_DATABASE_USERNAME`
- `TEST_DATABASE_PASSWORD`

### 3. 必要なAWSリソース

テストを実行する前に、以下のリソースが必要です：

**必須リソース**：
- **VPC**: テスト用のVPC
- **プライベートサブネット**（複数AZ）: ECSタスク配置用
- **パブリックサブネット**（複数AZ）: ALB配置用、NAT Gateway配置用（新規NAT Gateway作成時）
- **NAT Gateway**: **必須**
  - テストでは既存のNAT Gatewayを使用、または新規作成します
  - Public ECR (public.ecr.aws) からのイメージプルとBaseMachina認証サーバーへのアクセスに必要
  - プライベートサブネットのルートテーブルに0.0.0.0/0 → NAT Gatewayのルートが設定されていること
- **BaseMachinaテナントID**: Bridge設定用

**自動作成されるリソース**（テストで毎回作成）:
- **VPCエンドポイント**: ECR API、ECR Docker、S3、CloudWatch Logs
  - Private ECRアクセス用、コスト削減とセキュリティ向上
- **ECRプルスルーキャッシュ**: Public ECR → Private ECRキャッシュルール
  - Public ECRイメージの可用性向上

**Route53 Hosted Zone（必須）**：
- 既存のRoute53 Hosted Zone
- ドメイン名と一致するZone ID
- Route53への書き込み権限

Hosted Zoneがない場合は、以下のコマンドで作成できます：

```bash
# Hosted Zoneを作成
aws route53 create-hosted-zone \
  --name example.com \
  --caller-reference $(date +%s)

# Zone IDを確認
aws route53 list-hosted-zones \
  --query "HostedZones[?Name=='example.com.'].Id" \
  --output text
```

出力例: `Z1234567890ABC`

**注**: Terratestでは、Bridge単体のHTTPS疎通確認のみを実施します。RDSインスタンスは作成せず、以下のRDS関連環境変数は不要です：
- `TEST_DATABASE_USERNAME`
- `TEST_DATABASE_PASSWORD`

### 4. terraform.tfvarsファイル

`examples/aws-ecs-fargate/terraform.tfvars`を作成してください：

```bash
cd examples/aws-ecs-fargate
cp terraform.tfvars.example terraform.tfvars
# terraform.tfvarsを編集して実際の値を設定
```

## テストの実行

### すべてのテストを実行

```bash
cd test
go test -v ./aws -timeout 60m
```

### 特定のテストを実行

```bash
cd test
go test -v ./aws -run TestECSFargateModule -timeout 60m
```

## テストの内容

### TestECSFargateModule

このテストは以下を検証します：

1. **事前検証**
   - Route53 Hosted Zoneの存在確認

2. **モジュールのデプロイ成功**
   - `terraform init`と`terraform apply`が成功すること

3. **リソース作成**
   - ECS Cluster、Task Definition、Service
   - ALB (Application Load Balancer)
   - NAT Gateway（Public ECRアクセスとBaseMachina認証サーバー接続用、必須）
   - VPC Endpoints (ECR API/DKR, S3, CloudWatch Logs)（常に作成）
   - ECR Pull Through Cache（Public ECR → Private ECRキャッシュ）
   - ACM Certificate（DNS検証で自動発行、最大15分タイムアウト）
   - Route53 A Record（ALBへのエイリアス）

4. **出力値の確認**
   - すべての出力値（ALB、ECS、IAM等）が空でないこと

5. **ECSサービスの状態**
   - ECSサービスが`desired_count`の数のタスクを実行していること（最大5分待機）

6. **ALBヘルスチェック**
   - ALBのターゲットグループでヘルスチェックがhealthyであること（最大5分待機）

7. **HTTPS エンドポイントテスト**
   - `https://[DOMAIN]/ok`へのHTTPSリクエストが成功すること（DNS検証で発行されたACM証明書を使用）
   - HTTPステータスコード200が返されること
   - 最大10分間、10秒間隔でリトライを実行

8. **自動クリーンアップ**
   - テスト終了後に`terraform destroy`で自動的にリソースが削除されること
   - Route53レコード（A、CNAMEレコード）も自動削除

**注**: RDS接続テストはTerratestでは実施しません。Bridge単体のHTTPS疎通確認のみ行います。

## テストの流れ

1. **事前検証**: Route53 Hosted Zoneの存在確認
2. **初期化**: Terraformで環境を初期化
3. **リソース作成**:
   - ECS Cluster、Task Definition、Service
   - ALB (Application Load Balancer)
   - NAT Gateway（Public ECRアクセスとBaseMachina認証サーバー接続用、必須）
   - VPC Endpoints (ECR API/DKR, S3, CloudWatch Logs)（常に作成）
   - ECR Pull Through Cache（Public ECR → Private ECRキャッシュ）
   - ACM Certificate（DNS検証で自動発行）
   - Route53 A Record（ALBへのエイリアス）
4. **ヘルスチェック**:
   - ECSタスクの起動確認（最大5分待機）
   - ALBターゲットグループのヘルスチェック（最大5分待機）
   - HTTPS エンドポイントの疎通確認（最大10分待機）
5. **クリーンアップ**: terraform destroyでリソースを削除

## 実行時間

テストの実行には約15〜20分かかります：

- Route53検証: 30秒
- ACM証明書のDNS検証: 5〜10分（タイムアウト: 15分）
- Bridge初期化: 2〜5分
- その他のリソース作成: 5分
- Terraform destroy: 2〜3分

**注意**: ACM証明書のDNS検証が15分以内に完了しない場合、テストは失敗します。

## トラブルシューティング

### ACM証明書の検証が完了しない

DNS検証レコードが正しく作成されているか確認してください：

```bash
# 診断スクリプトを実行（推奨）
cd ../examples/aws-ecs-fargate
./scripts/diagnose-dns-validation.sh $TEST_BRIDGE_DOMAIN_NAME $TEST_ROUTE53_ZONE_ID

# または手動で確認
# ACM証明書のステータス確認
aws acm describe-certificate \
  --certificate-arn arn:aws:acm:REGION:ACCOUNT:certificate/CERT_ID \
  --query 'Certificate.Status'

# Route53のレコード確認
aws route53 list-resource-record-sets \
  --hosted-zone-id $TEST_ROUTE53_ZONE_ID \
  --query "ResourceRecordSets[?Type=='CNAME']"

# DNS検証レコードの確認
dig _acm-challenge.$TEST_BRIDGE_DOMAIN_NAME CNAME
```

**よくある原因**:
1. **Zone IDが間違っている**: ドメイン名と一致するZone IDか確認
2. **ドメインが別のZoneに属している**: `aws.bm-tftest.com`は`bm-tftest.com`のZoneが必要
3. **DNSの伝播待ち**: 初回は5-10分、通常は1-2分必要
4. **権限不足**: Route53への書き込み権限を確認

### ECSタスクが起動しない（0 running tasks）

デフォルトでVPCエンドポイントを使用しているため、通常は問題なく起動します。起動しない場合は以下を確認してください：

1. **VPCエンドポイントの作成状態を確認**（デフォルト構成の場合）：
   ```bash
   # ECR API エンドポイント
   aws ec2 describe-vpc-endpoints \
     --filters "Name=service-name,Values=com.amazonaws.ap-northeast-1.ecr.api" \
     --query 'VpcEndpoints[0].State'

   # ECR Docker エンドポイント
   aws ec2 describe-vpc-endpoints \
     --filters "Name=service-name,Values=com.amazonaws.ap-northeast-1.ecr.dkr" \
     --query 'VpcEndpoints[0].State'
   ```
   - `State`が`available`であること

2. **NAT Gatewayの設定を確認**（必須）：
   ```bash
   # プライベートサブネットのルートテーブルを確認
   aws ec2 describe-route-tables \
     --filters "Name=association.subnet-id,Values=subnet-xxxxx" \
     --query 'RouteTables[0].Routes[?DestinationCidrBlock==`0.0.0.0/0`]'
   ```
   - `NatGatewayId`が存在すれば正常

3. **ECSタスクのエラーログを確認**：
   テストログに`Stopped task reason`と`Container reason`が表示されます

**よくある原因**：
- VPCエンドポイントのセキュリティグループでHTTPS（443）通信が許可されていない
- プライベートサブネットのルートテーブルにS3ゲートウェイエンドポイントが設定されていない
- NAT Gatewayがプライベートサブネットのルートテーブルに設定されていない（**Public ECRアクセスに必須**）
- NAT Gatewayが存在しない（**Public ECRアクセスに必須**）

### テストがタイムアウトする

デフォルトのタイムアウトは60分です（DNS検証を考慮）。長い場合は`-timeout`フラグを調整してください：

```bash
go test -v ./aws -timeout 90m
```

### AWS認証エラー

```
Error: error configuring Terraform AWS Provider: no valid credential sources
```

AWS認証情報が正しく設定されているか確認してください。

### リソースが残る

テストが異常終了した場合、AWSリソースが残る可能性があります。手動でクリーンアップしてください：

```bash
cd examples/aws-ecs-fargate
terraform destroy
```

## CI/CD統合

GitHub ActionsなどのCI/CDパイプラインで実行する場合：

```yaml
- name: Run Terratest
  env:
    AWS_ACCESS_KEY_ID: ${{ secrets.AWS_ACCESS_KEY_ID }}
    AWS_SECRET_ACCESS_KEY: ${{ secrets.AWS_SECRET_ACCESS_KEY }}
    AWS_DEFAULT_REGION: ap-northeast-1
    TEST_VPC_ID: ${{ secrets.TEST_VPC_ID }}
    TEST_PRIVATE_SUBNET_IDS: ${{ secrets.TEST_PRIVATE_SUBNET_IDS }}
    TEST_PUBLIC_SUBNET_IDS: ${{ secrets.TEST_PUBLIC_SUBNET_IDS }}
    TEST_TENANT_ID: ${{ secrets.TEST_TENANT_ID }}
    TEST_BRIDGE_DOMAIN_NAME: ${{ secrets.TEST_BRIDGE_DOMAIN_NAME }}
    TEST_ROUTE53_ZONE_ID: ${{ secrets.TEST_ROUTE53_ZONE_ID }}
  run: |
    cd test
    go test -v ./aws -timeout 60m
```

## 注意事項

1. **コスト**: テスト実行には以下のAWSリソースが作成されます：
   - NAT Gateway: 約$0.045/時間 + データ転送料（**必須**）
   - ALB: 約$0.0225/時間
   - ECS Fargate: vCPU/メモリ使用量に応じた課金
   - VPC Endpoints: 約$0.01/時間/エンドポイント（デフォルト構成）
   - Route53: クエリ数に応じた課金（Hosted Zoneは$0.50/月）

2. **並列実行**: 複数のテストを並列実行する場合、`TEST_BRIDGE_DOMAIN_NAME`にユニークな値を設定してください（例: `bridge-test-1.example.com`, `bridge-test-2.example.com`）

3. **Hosted Zoneの管理**: テストではHosted Zone自体は作成・削除しません。事前に作成し、テスト後も残しておいてください。Route53レコード（A、CNAMEレコード）は自動的にクリーンアップされます。

4. **証明書のキャッシュ**: DNS検証によるACM証明書は自動的に作成されますが、同じドメインで複数回テストを実行する場合、証明書の検証時間が短縮されることがあります。

5. **テストは並列実行可能**: `t.Parallel()`を使用しています

6. **テスト用のリソース**: 一意のプレフィックスが自動的に付与されます

7. **テスト失敗時**: ログを確認してトラブルシューティングを行ってください

## Google Cloud Cloud Runテスト

Google Cloud Cloud Runモジュールの統合テストです。[Terratest](https://terratest.gruntwork.io/)を使用して実装されており、実際のGoogle Cloudリソースをデプロイして検証します。

### テスト内容

このテストスイートは以下を検証します：

1. **Cloud Run Service検証**
   - Cloud Runサービスの存在確認
   - 環境変数の設定確認（FETCH_INTERVAL、FETCH_TIMEOUT、PORT、TENANT_ID）
   - リソース制限の確認（CPU、メモリ）
   - Ingress設定の確認（内部ロードバランサーのみ）

2. **HTTPS疎通とヘルスチェック**
   - `/ok`エンドポイントへのHTTPSリクエスト
   - HTTPステータスコード200の確認
   - レスポンスボディの検証
   - SSL証明書の有効性確認
   - SSL証明書発行とDNS伝播の待機（最大20分）

3. **Cloud SQL接続テスト**
   - Cloud SQLインスタンスの存在確認
   - プライベートIP設定の検証
   - パブリックIP無効化の確認
   - バックアップ設定の検証（point-in-time recovery）

4. **DNS解決とLoad Balancerテスト**
   - DNSルックアップによるAレコード検証
   - Load Balancer IPアドレスとの一致確認
   - Cloud Armorアクセス制御の動作確認

### Google Cloudテスト前提条件

#### 1. Google Cloudプロジェクト

有効なGoogle Cloudプロジェクトと以下の有効化されたAPI：

```bash
gcloud services enable run.googleapis.com
gcloud services enable compute.googleapis.com
gcloud services enable sqladmin.googleapis.com
gcloud services enable servicenetworking.googleapis.com
gcloud services enable dns.googleapis.com  # DNS統合を使用する場合
```

#### 2. 認証情報

Google Cloudへの認証を設定します：

```bash
# サービスアカウントを使用する場合
export GOOGLE_APPLICATION_CREDENTIALS="/path/to/service-account-key.json"

# またはgcloud CLIで認証
gcloud auth application-default login
```

サービスアカウントには以下の権限が必要です：
- Cloud Run Admin
- Compute Admin
- Cloud SQL Admin
- Service Networking Admin
- DNS Administrator（DNS統合を使用する場合）

#### 3. Cloud DNS Managed Zone（オプション）

HTTPS/DNSテストを実行する場合、事前にCloud DNS Managed Zoneを作成してください：

```bash
gcloud dns managed-zones create example-com \
  --dns-name="example.com." \
  --description="Test domain"
```

### Google Cloud環境変数

#### 必須環境変数

| 変数名 | 説明 | 例 |
|--------|------|-----|
| `TEST_GCP_PROJECT_ID` | テスト用のGoogle CloudプロジェクトID | `my-test-project` |
| `TEST_TENANT_ID` | BaseMachinaテナントID | `tenant-123` |

#### オプション環境変数

| 変数名 | 説明 | デフォルト | 例 |
|--------|------|-----------|-----|
| `TEST_GCP_REGION` | Google Cloudリージョン | `asia-northeast1` | `us-central1` |
| `TEST_DOMAIN_NAME` | カスタムドメイン名（HTTPS/DNSテスト用） | なし | `bridge-test.example.com` |
| `TEST_DNS_ZONE_NAME` | Cloud DNS Managed Zone名 | なし | `example-com` |

#### 環境変数設定例

**HTTPのみのテスト（最小構成）**:

```bash
export TEST_GCP_PROJECT_ID="my-test-project"
export TEST_TENANT_ID="tenant-123"
export TEST_GCP_REGION="asia-northeast1"
```

**HTTPS/DNSを含む完全なテスト**:

```bash
export TEST_GCP_PROJECT_ID="my-test-project"
export TEST_TENANT_ID="tenant-123"
export TEST_GCP_REGION="asia-northeast1"
export TEST_DOMAIN_NAME="bridge-test.example.com"
export TEST_DNS_ZONE_NAME="example-com"
```

### Google Cloudテスト実行手順

#### 1. 環境変数の設定

上記の必須環境変数を設定してください。

#### 2. テストの実行

プロジェクトルートの`test`ディレクトリから実行します：

```bash
cd test
go test -v ./gcp -timeout 60m
```

**注意**: テストには最大60分かかる場合があります（SSL証明書のプロビジョニング、DNS伝播、リソース作成を含む）。

#### 3. 特定のテストのみ実行

```bash
# Cloud Runサービス検証のみ
go test -v ./gcp -run TestCloudRunModule/CloudRunServiceExists -timeout 30m

# HTTPS疎通テストのみ
go test -v ./gcp -run TestCloudRunModule/HTTPSHealthCheck -timeout 30m

# Cloud SQL接続テストのみ
go test -v ./gcp -run TestCloudRunModule/CloudSQLInstanceExists -timeout 30m

# DNS解決テストのみ
go test -v ./gcp -run TestCloudRunModule/DNSResolutionAndLoadBalancer -timeout 30m
```

### Google Cloudテスト実行時の注意事項

#### タイムアウト

- **SSL証明書プロビジョニング**: 最大15分
- **DNS伝播**: 最大5分
- **Cloud SQLインスタンス作成**: 最大10分
- **terraform apply**: 全体で15-20分

合計で最大60分のタイムアウトを推奨します。

#### リソースクリーンアップ

テストは`defer terraform.Destroy(t, terraformOptions)`を使用して、自動的にリソースをクリーンアップします。ただし、VPC Peering削除の既知の問題により、terraform destroyが失敗する場合があります。

**推奨されるクリーンアップ方法（最速）**:

```bash
cd examples/gcp-cloud-run
./scripts/quick-cleanup.sh YOUR_PROJECT_ID basemachina-bridge-example
```

このスクリプトは：
1. まず`terraform destroy`を試行
2. 失敗した場合、VPCネットワークを直接削除（VPC Peeringも一緒に削除される）
3. 実行時間: 約1-2分

**完全なクリーンアップ（すべてのリソースを確認）**:

```bash
cd examples/gcp-cloud-run
./scripts/cleanup.sh YOUR_PROJECT_ID basemachina-bridge-example
```

このスクリプトは以下を順番に削除します（時間がかかります）：
1. Cloud Run サービス
2. Cloud SQL インスタンス（最大10分）
3. VPC Peering 接続
4. Load Balancer リソース
5. VPC ネットワークとサブネット

**Terraformによるクリーンアップ**:

```bash
cd examples/gcp-cloud-run
terraform destroy
```

**注意**: terraform destroyはVPC Peering削除エラーで失敗する可能性があります。その場合は上記のquick-cleanup.shスクリプトを使用してください。

**手動クリーンアップ（Google Cloudコンソール）**:

Google Cloudコンソールから以下のリソースを手動で削除：
- Cloud Runサービス: `bridge-test-*` または `basemachina-bridge-example`
- Cloud SQLインスタンス: `bridge-test-*-db-*` または `basemachina-bridge-example-db-*`
- Load Balancer関連リソース
- VPC ネットワーク: `bridge-test-*-vpc` または `basemachina-bridge-example-vpc`

#### コスト

テストは実際のGoogle Cloudリソースを作成するため、以下のコストが発生します：

- Cloud Run: 実行時間に応じた課金（無料枠あり）
- Cloud SQL: インスタンス実行時間（db-f1-micro: 約$10/月）
- Cloud Load Balancer: 転送量に応じた課金
- VPC Egress: データ転送量

テスト実行時間は通常30-60分で、コストは$1-5程度です。

### Google Cloudトラブルシューティング

#### SSL証明書のプロビジョニングが完了しない

**症状**: `HTTPSHealthCheck`テストが20分後にタイムアウト

**原因**:
- DNSレコードが正しく設定されていない
- DNS Managed Zoneが存在しない
- ドメイン名のネームサーバーがCloud DNSを指していない

**解決方法**:
1. Cloud DNS Managed Zoneの存在確認:
   ```bash
   gcloud dns managed-zones describe example-com
   ```

2. ネームサーバーの確認:
   ```bash
   gcloud dns managed-zones describe example-com --format="value(nameServers)"
   ```

3. ドメインレジストラで、上記のネームサーバーを設定

4. DNSレコードの確認:
   ```bash
   dig +short bridge-test.example.com
   ```

#### Cloud SQLインスタンス作成エラー

**症状**: `Error creating sql database instance`

**原因**:
- Service Networking APIが有効化されていない
- VPCピアリング用のIPアドレス範囲が不足

**解決方法**:
1. Service Networking APIの有効化:
   ```bash
   gcloud services enable servicenetworking.googleapis.com
   ```

2. プロジェクトのクォータ確認:
   ```bash
   gcloud compute project-info describe --project=PROJECT_ID
   ```

#### VPCピアリング接続エラー

**症状**: `Error creating service networking connection`

**原因**:
- 既存のVPCピアリング接続と競合
- IPアドレス範囲の重複

**解決方法**:
1. 既存のピアリング接続を確認:
   ```bash
   gcloud services vpc-peerings list --service=servicenetworking.googleapis.com
   ```

2. 競合する接続を削除:
   ```bash
   gcloud services vpc-peerings delete \
     --service=servicenetworking.googleapis.com \
     --network=NETWORK_NAME
   ```

#### VPC Peering削除エラー（terraform destroy時）

**症状**: `Error: Unable to remove Service Networking Connection, err: Error waiting for Delete Service Networking Connection: Error code 9, message: Failed to delete connection; Producer services (e.g. CloudSQL, Cloud Memstore, etc.) are still using this connection.`

**原因**:
- Cloud SQLインスタンスの削除が完了する前にVPC Peering接続の削除を試みている
- Google CloudのAPI側で削除処理が非同期で行われるため、タイミング問題が発生

**解決方法**:

1. **自動的に解決される場合がほとんどです**。`deletion_policy = "ABANDON"`が設定されているため、VPCネットワーク全体が削除されると、VPC Peering接続も自動的にクリーンアップされます。

2. **手動でVPC Peering接続を削除する場合**:
   ```bash
   # 既存のピアリング接続を確認
   gcloud services vpc-peerings list --service=servicenetworking.googleapis.com --network=NETWORK_NAME

   # VPC Peering接続を削除（Cloud SQLインスタンスが削除された後）
   gcloud services vpc-peerings delete \
     --service=servicenetworking.googleapis.com \
     --network=NETWORK_NAME
   ```

3. **テスト失敗時のリソースクリーンアップ**:
   ```bash
   # Cloud SQLインスタンスを手動で削除
   gcloud sql instances delete INSTANCE_NAME --project=PROJECT_ID

   # VPCネットワークを削除（VPC Peering接続も一緒に削除されます）
   gcloud compute networks delete NETWORK_NAME --project=PROJECT_ID
   ```

**注意**: このエラーはテスト環境で時々発生しますが、リソースは最終的にクリーンアップされます。継続的にエラーが発生する場合は、上記の手動削除手順を実行してください。

#### テスト実行時の権限エラー

**症状**: `Error 403: Permission denied`

**原因**: サービスアカウントに必要な権限がない

**解決方法**:
サービスアカウントに以下のロールを付与:
```bash
gcloud projects add-iam-policy-binding PROJECT_ID \
  --member="serviceAccount:SERVICE_ACCOUNT_EMAIL" \
  --role="roles/run.admin"

gcloud projects add-iam-policy-binding PROJECT_ID \
  --member="serviceAccount:SERVICE_ACCOUNT_EMAIL" \
  --role="roles/compute.admin"

gcloud projects add-iam-policy-binding PROJECT_ID \
  --member="serviceAccount:SERVICE_ACCOUNT_EMAIL" \
  --role="roles/cloudsql.admin"
```

#### DNSルックアップ失敗

**症状**: `DNS lookup failed: no such host`

**原因**:
- DNS伝播が完了していない
- Aレコードが作成されていない

**解決方法**:
1. Cloud DNSでAレコードの存在確認:
   ```bash
   gcloud dns record-sets list --zone=example-com
   ```

2. NSレコードの確認:
   ```bash
   dig NS example.com
   ```

3. 外部DNSサーバーでの確認:
   ```bash
   dig @8.8.8.8 bridge-test.example.com
   ```

#### Cloud Armorによるアクセス拒否

**症状**: `HTTP 403 Forbidden`

**原因**: テスト実行元のIPアドレスが許可リストに含まれていない

**解決方法**:
1. 現在のIPアドレスを確認:
   ```bash
   curl ifconfig.me
   ```

2. `terraform.tfvars`に自分のIPアドレスを追加:
   ```hcl
   allowed_ip_ranges = ["34.85.43.93/32", "YOUR_IP/32"]
   ```

### Google Cloud CI/CD統合

GitHub Actionsでテストを実行する場合の例：

```yaml
name: Google Cloud Run Tests

on:
  push:
    branches: [ main ]
  pull_request:
    branches: [ main ]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3

      - name: Set up Go
        uses: actions/setup-go@v4
        with:
          go-version: '1.21'

      - name: Set up gcloud
        uses: google-github-actions/setup-gcloud@v1
        with:
          service_account_key: ${{ secrets.GCP_SA_KEY }}
          project_id: ${{ secrets.GCP_PROJECT_ID }}

      - name: Run tests
        env:
          TEST_GCP_PROJECT_ID: ${{ secrets.GCP_PROJECT_ID }}
          TEST_TENANT_ID: ${{ secrets.TENANT_ID }}
          TEST_DOMAIN_NAME: ${{ secrets.TEST_DOMAIN_NAME }}
          TEST_DNS_ZONE_NAME: ${{ secrets.TEST_DNS_ZONE_NAME }}
        run: |
          cd test
          go test -v ./gcp -timeout 60m
```

## 参考資料

### AWS
- [Terratest公式ドキュメント](https://terratest.gruntwork.io/)
- [AWS ECS Fargate料金](https://aws.amazon.com/jp/fargate/pricing/)
- [AWS Route53料金](https://aws.amazon.com/jp/route53/pricing/)
- [AWS Certificate Manager料金](https://aws.amazon.com/jp/certificate-manager/pricing/)

### Google Cloud
- [Cloud Run Testing Best Practices](https://cloud.google.com/run/docs/testing)
- [Cloud SQL Testing](https://cloud.google.com/sql/docs/mysql/testing)
- [Google Cloud Go SDK](https://cloud.google.com/go/docs/reference)
- [Cloud Run料金](https://cloud.google.com/run/pricing)
- [Cloud SQL料金](https://cloud.google.com/sql/pricing)
