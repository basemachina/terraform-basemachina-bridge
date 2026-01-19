#!/bin/bash
#
# Google Cloud リソース簡易クリーンアップスクリプト
#
# VPCネットワークを削除すると、VPC Peeringも一緒に削除されます。
# Cloud SQLの削除は時間がかかるため、このスクリプトでは削除を開始するだけです。
#
# 使用方法:
#   ./quick-cleanup.sh <project-id> <service-name-prefix>
#
# 例:
#   ./quick-cleanup.sh my-gcp-project basemachina-bridge-example
#

set -e

PROJECT_ID=$1
SERVICE_NAME=${2:-"basemachina-bridge-example"}

if [ -z "$PROJECT_ID" ]; then
    echo "使用方法: $0 <project-id> [service-name]"
    exit 1
fi

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "簡易クリーンアップ"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "プロジェクト: $PROJECT_ID"
echo "サービス名: $SERVICE_NAME"
echo ""

# まず terraform destroy を試みる
echo "ステップ 1: terraform destroy を実行..."
echo ""

if terraform destroy -auto-approve 2>&1 | tee /tmp/terraform-destroy.log; then
    echo ""
    echo "✅ terraform destroy が成功しました"
    echo ""
    echo "すべてのリソースが削除されました。"
    exit 0
else
    echo ""
    echo "⚠️  terraform destroy が失敗しました（VPC Peering削除エラーの可能性）"
    echo ""
fi

# VPCネットワークを直接削除
echo "ステップ 2: VPCネットワークを直接削除..."
echo ""

NETWORK_NAME="${SERVICE_NAME}-vpc"
echo "  削除中: $NETWORK_NAME"

# VPC Peeringを先に削除
echo "  VPC Peeringを削除中..."
gcloud services vpc-peerings delete \
    --service=servicenetworking.googleapis.com \
    --network=$NETWORK_NAME \
    --project=$PROJECT_ID \
    --quiet 2>/dev/null || true

# VPC Peering削除完了まで待機
echo "  VPC Peering削除完了を待機中（30秒）..."
sleep 30

# サブネットを削除
echo "  サブネットを削除中..."
gcloud compute networks subnets list --project=$PROJECT_ID --network=$NETWORK_NAME --format="value(name,region)" 2>/dev/null | while read subnet region; do
    if [ -n "$subnet" ] && [ -n "$region" ]; then
        echo "    削除: $subnet (リージョン: $region)"
        gcloud compute networks subnets delete $subnet --project=$PROJECT_ID --region=$region --quiet || true
    fi
done

# グローバルアドレスを削除
echo "  グローバルアドレスを削除中..."
GLOBAL_ADDRESS_NAME="${SERVICE_NAME}-private-ip"
gcloud compute addresses delete $GLOBAL_ADDRESS_NAME --project=$PROJECT_ID --global --quiet 2>/dev/null || true

# VPCネットワークを削除
echo "  VPCネットワークを削除中..."
gcloud compute networks delete $NETWORK_NAME --project=$PROJECT_ID --quiet || true

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ クリーンアップ完了"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "注意:"
echo "- Cloud SQLインスタンスがまだ削除中の場合があります"
echo "- Google Cloudコンソールで削除の完了を確認してください"
echo ""
echo "残っているCloud SQLインスタンスを確認:"
echo "  gcloud sql instances list --project=$PROJECT_ID --filter=\"name:$SERVICE_NAME\""
echo ""
echo "手動で削除する場合:"
echo "  gcloud sql instances delete INSTANCE_NAME --project=$PROJECT_ID"
echo ""
