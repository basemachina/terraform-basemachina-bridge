#!/bin/bash
#
# Google Cloud リソース強制クリーンアップスクリプト
#
# 使用方法:
#   ./cleanup.sh <project-id> [service-name-prefix]
#
# 例:
#   ./cleanup.sh my-gcp-project basemachina-bridge-example
#   ./cleanup.sh my-gcp-project  # デフォルト: basemachina-bridge-example
#

set -e

# ========================================
# 引数チェック
# ========================================

if [ $# -lt 1 ]; then
    echo "使用方法: $0 <project-id> [service-name-prefix]"
    echo ""
    echo "例:"
    echo "  $0 my-gcp-project basemachina-bridge-example"
    echo "  $0 my-gcp-project  # デフォルト: basemachina-bridge-example"
    exit 1
fi

PROJECT_ID=$1
SERVICE_NAME_PREFIX=${2:-"basemachina-bridge-example"}

echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Google Cloud リソース強制クリーンアップ"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "プロジェクト: $PROJECT_ID"
echo "サービス名プレフィックス: $SERVICE_NAME_PREFIX"
echo ""

# ========================================
# 確認
# ========================================

read -p "本当にリソースを削除しますか？ (yes/no): " CONFIRM
if [ "$CONFIRM" != "yes" ]; then
    echo "キャンセルしました。"
    exit 0
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "ステップ 1: Cloud Run サービスの削除"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Cloud Runサービスを検索して削除
for region in asia-northeast1 us-central1 europe-west1; do
    echo "リージョン $region のCloud Runサービスを確認中..."
    SERVICES=$(gcloud run services list --project=$PROJECT_ID --region=$region --format="value(metadata.name)" --filter="metadata.name:${SERVICE_NAME_PREFIX}*" 2>/dev/null || true)

    if [ -n "$SERVICES" ]; then
        for service in $SERVICES; do
            echo "  削除中: $service (リージョン: $region)"
            gcloud run services delete $service --project=$PROJECT_ID --region=$region --quiet || true
        done
    fi
done

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "ステップ 2: Cloud SQL インスタンスの削除"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Cloud SQLインスタンスを検索して削除
echo "  Cloud SQLインスタンスを検索中..."
SQL_INSTANCES=$(timeout 30 gcloud sql instances list --project=$PROJECT_ID --format="value(name)" --filter="name:${SERVICE_NAME_PREFIX}*" 2>/dev/null || true)

if [ -n "$SQL_INSTANCES" ]; then
    for instance in $SQL_INSTANCES; do
        echo "  削除を開始: $instance"
        echo "  注意: Cloud SQL削除には5-10分かかる場合があります"

        # バックグラウンドで削除を開始
        gcloud sql instances delete $instance --project=$PROJECT_ID --quiet &
        DELETE_PID=$!

        # 削除の進捗を表示
        echo "  削除中（PID: $DELETE_PID）... 最大10分待機します"

        # 10分間待機（削除が完了するまで）
        for i in {1..20}; do
            if ps -p $DELETE_PID > /dev/null 2>&1; then
                echo "  ... ${i}0秒経過（削除進行中）"
                sleep 30
            else
                echo "  削除完了"
                break
            fi
        done

        # プロセスがまだ実行中の場合は強制終了
        if ps -p $DELETE_PID > /dev/null 2>&1; then
            echo "  警告: 削除が10分経過しても完了しませんでした"
            echo "  バックグラウンドで削除が続行されます"
            kill $DELETE_PID 2>/dev/null || true
        fi
    done

    # 追加の待機時間
    echo "  削除完了を確認中（30秒）..."
    sleep 30
else
    echo "  Cloud SQLインスタンスが見つかりませんでした。"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "ステップ 3: VPC Peering 接続の削除"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# VPCネットワークを検索
NETWORKS=$(gcloud compute networks list --project=$PROJECT_ID --format="value(name)" --filter="name:${SERVICE_NAME_PREFIX}*" 2>/dev/null || true)

if [ -n "$NETWORKS" ]; then
    for network in $NETWORKS; do
        echo "  VPC Peering接続を削除中: $network"
        gcloud services vpc-peerings delete \
            --service=servicenetworking.googleapis.com \
            --network=$network \
            --project=$PROJECT_ID \
            --quiet || true

        # VPC Peering削除完了まで待機
        echo "  VPC Peering削除完了を待機中（30秒）..."
        sleep 30
    done
else
    echo "  VPCネットワークが見つかりませんでした。"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "ステップ 4: Load Balancer の削除（依存関係順）"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "  Load Balancerリソースを依存関係の順序で削除します："
echo "  1. Forwarding Rules → 2. Target Proxies → 3. URL Maps → 4. SSL Certificates"
echo "  5. Backend Services → 6. Security Policies → 7. NEGs → 8. Global Addresses"
echo ""

# 1. Forwarding Ruleを削除（最上位レベル）
echo "  [1/8] Forwarding Rulesを削除中..."
FORWARDING_RULES=$(gcloud compute forwarding-rules list --project=$PROJECT_ID --global --format="value(name)" --filter="name:${SERVICE_NAME_PREFIX}*" 2>/dev/null || true)
if [ -n "$FORWARDING_RULES" ]; then
    for rule in $FORWARDING_RULES; do
        echo "    - 削除中: Forwarding Rule $rule"
        gcloud compute forwarding-rules delete $rule --project=$PROJECT_ID --global --quiet || true
    done
else
    echo "    (Forwarding Rulesが見つかりませんでした)"
fi

# 2. Target HTTPS/HTTP Proxyを削除
echo "  [2/8] Target Proxiesを削除中..."
HTTPS_PROXIES=$(gcloud compute target-https-proxies list --project=$PROJECT_ID --format="value(name)" --filter="name:${SERVICE_NAME_PREFIX}*" 2>/dev/null || true)
if [ -n "$HTTPS_PROXIES" ]; then
    for proxy in $HTTPS_PROXIES; do
        echo "    - 削除中: Target HTTPS Proxy $proxy"
        gcloud compute target-https-proxies delete $proxy --project=$PROJECT_ID --global --quiet || true
    done
else
    echo "    (HTTPS Proxiesが見つかりませんでした)"
fi

HTTP_PROXIES=$(gcloud compute target-http-proxies list --project=$PROJECT_ID --format="value(name)" --filter="name:${SERVICE_NAME_PREFIX}*" 2>/dev/null || true)
if [ -n "$HTTP_PROXIES" ]; then
    for proxy in $HTTP_PROXIES; do
        echo "    - 削除中: Target HTTP Proxy $proxy"
        gcloud compute target-http-proxies delete $proxy --project=$PROJECT_ID --global --quiet || true
    done
else
    echo "    (HTTP Proxiesが見つかりませんでした)"
fi

# 3. URL Mapを削除
echo "  [3/8] URL Mapsを削除中..."
URL_MAPS=$(gcloud compute url-maps list --project=$PROJECT_ID --format="value(name)" --filter="name:${SERVICE_NAME_PREFIX}*" 2>/dev/null || true)
if [ -n "$URL_MAPS" ]; then
    for map in $URL_MAPS; do
        echo "    - 削除中: URL Map $map"
        gcloud compute url-maps delete $map --project=$PROJECT_ID --global --quiet || true
    done
else
    echo "    (URL Mapsが見つかりませんでした)"
fi

# 4. SSL証明書を削除
echo "  [4/8] SSL Certificatesを削除中..."
SSL_CERTS=$(gcloud compute ssl-certificates list --project=$PROJECT_ID --format="value(name)" --filter="name:${SERVICE_NAME_PREFIX}*" 2>/dev/null || true)
if [ -n "$SSL_CERTS" ]; then
    for cert in $SSL_CERTS; do
        echo "    - 削除中: SSL Certificate $cert"
        gcloud compute ssl-certificates delete $cert --project=$PROJECT_ID --global --quiet || true
    done
else
    echo "    (SSL Certificatesが見つかりませんでした)"
fi

# 5. Backend Serviceを削除
echo "  [5/8] Backend Servicesを削除中..."
BACKEND_SERVICES=$(gcloud compute backend-services list --project=$PROJECT_ID --global --format="value(name)" --filter="name:${SERVICE_NAME_PREFIX}*" 2>/dev/null || true)
if [ -n "$BACKEND_SERVICES" ]; then
    for service in $BACKEND_SERVICES; do
        echo "    - 削除中: Backend Service $service"
        gcloud compute backend-services delete $service --project=$PROJECT_ID --global --quiet || true
    done
else
    echo "    (Backend Servicesが見つかりませんでした)"
fi

# 6. Security Policyを削除
echo "  [6/8] Security Policiesを削除中..."
SECURITY_POLICIES=$(gcloud compute security-policies list --project=$PROJECT_ID --format="value(name)" --filter="name:${SERVICE_NAME_PREFIX}*" 2>/dev/null || true)
if [ -n "$SECURITY_POLICIES" ]; then
    for policy in $SECURITY_POLICIES; do
        echo "    - 削除中: Security Policy $policy"
        gcloud compute security-policies delete $policy --project=$PROJECT_ID --quiet || true
    done
else
    echo "    (Security Policiesが見つかりませんでした)"
fi

# 7. Network Endpoint Groupを削除
echo "  [7/8] Network Endpoint Groupsを削除中..."
NEG_FOUND=false
for region in asia-northeast1 us-central1 europe-west1; do
    NEGS=$(gcloud compute network-endpoint-groups list --project=$PROJECT_ID --regions=$region --format="value(name)" --filter="name:${SERVICE_NAME_PREFIX}*" 2>/dev/null || true)
    if [ -n "$NEGS" ]; then
        NEG_FOUND=true
        for neg in $NEGS; do
            echo "    - 削除中: Network Endpoint Group $neg (リージョン: $region)"
            gcloud compute network-endpoint-groups delete $neg --project=$PROJECT_ID --region=$region --quiet || true
        done
    fi
done
if [ "$NEG_FOUND" = false ]; then
    echo "    (Network Endpoint Groupsが見つかりませんでした)"
fi

# 8. Global Addressを削除
echo "  [8/8] Global Addressesを削除中..."
ADDRESSES=$(gcloud compute addresses list --project=$PROJECT_ID --global --format="value(name)" --filter="name:${SERVICE_NAME_PREFIX}*" 2>/dev/null || true)
if [ -n "$ADDRESSES" ]; then
    for address in $ADDRESSES; do
        echo "    - 削除中: Global Address $address"
        gcloud compute addresses delete $address --project=$PROJECT_ID --global --quiet || true
    done
else
    echo "    (Global Addressesが見つかりませんでした)"
fi

echo ""
echo "  ✅ Load Balancerリソースの削除が完了しました"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "ステップ 5: Serverless VPC Access アドレスの削除"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Serverless IPv4アドレスを削除
# Cloud Run Direct VPC Egressが作成するserverless-ipv4アドレスを削除
echo "  Serverless IPv4アドレスを検索中..."
SERVERLESS_FOUND=false
for region in asia-northeast1 us-central1 europe-west1; do
    SERVERLESS_ADDRESSES=$(gcloud compute addresses list --project=$PROJECT_ID --regions=$region --format="value(name)" --filter="name~serverless-ipv4 AND subnetwork~${SERVICE_NAME_PREFIX}" 2>/dev/null || true)

    if [ -n "$SERVERLESS_ADDRESSES" ]; then
        SERVERLESS_FOUND=true
        for address in $SERVERLESS_ADDRESSES; do
            echo "    - 削除試行中: $address (リージョン: $region)"
            # serverless-ipv4は削除できない場合があるため、エラーを無視
            gcloud compute addresses delete $address --project=$PROJECT_ID --region=$region --quiet 2>/dev/null || {
                echo "      ⚠️  削除に失敗（Google Cloudが自動クリーンアップする可能性があります）"
            }
        done
    fi
done
if [ "$SERVERLESS_FOUND" = false ]; then
    echo "    (Serverless IPv4アドレスが見つかりませんでした)"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "ステップ 6: VPC ネットワークとサブネットの削除（依存関係順）"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "  VPCリソースを依存関係の順序で削除します："
echo "  1. Custom Routes → 2. Subnets → 3. VPC Networks"
echo ""

# VPCネットワークを先に取得（後で使用）
NETWORKS=$(gcloud compute networks list --project=$PROJECT_ID --format="value(name)" --filter="name:${SERVICE_NAME_PREFIX}*" 2>/dev/null || true)

# 1. カスタムルートを削除（デフォルトルート以外）
echo "  [1/3] Custom Routesを削除中..."
ROUTES_FOUND=false
if [ -n "$NETWORKS" ]; then
    for network in $NETWORKS; do
        # ローカルルート（default-route-r-*）は削除できないため、インターネットゲートウェイへのルートのみ削除
        CUSTOM_ROUTES=$(gcloud compute routes list --project=$PROJECT_ID --format="value(name)" --filter="network~${network} AND nextHopGateway:default-internet-gateway" 2>/dev/null || true)
        if [ -n "$CUSTOM_ROUTES" ]; then
            ROUTES_FOUND=true
            for route in $CUSTOM_ROUTES; do
                echo "    - 削除中: Route $route (ネットワーク: $network)"
                gcloud compute routes delete $route --project=$PROJECT_ID --quiet || {
                    echo "      ⚠️  削除に失敗（ローカルルートは削除できません）"
                }
            done
        fi
    done
fi
if [ "$ROUTES_FOUND" = false ]; then
    echo "    (削除可能なカスタムルートが見つかりませんでした)"
fi

# 2. サブネットを削除
echo "  [2/3] Subnetsを削除中..."
SUBNETS=$(gcloud compute networks subnets list --project=$PROJECT_ID --format="value(name,region)" --filter="name:${SERVICE_NAME_PREFIX}*" 2>/dev/null || true)
if [ -n "$SUBNETS" ]; then
    echo "$SUBNETS" | while read subnet region; do
        if [ -n "$subnet" ] && [ -n "$region" ]; then
            echo "    - 削除中: Subnet $subnet (リージョン: $region)"
            gcloud compute networks subnets delete $subnet --project=$PROJECT_ID --region=$region --quiet || {
                echo "      ⚠️  削除に失敗（serverless-ipv4が使用中の可能性があります）"
            }
        fi
    done
else
    echo "    (Subnetsが見つかりませんでした)"
fi

# 3. VPCネットワークを削除
echo "  [3/3] VPC Networksを削除中..."
if [ -n "$NETWORKS" ]; then
    for network in $NETWORKS; do
        echo "    - 削除中: VPC Network $network"
        gcloud compute networks delete $network --project=$PROJECT_ID --quiet || {
            echo "      ⚠️  削除に失敗（依存リソースが残っている可能性があります）"
        }
    done
else
    echo "    (VPC Networksが見つかりませんでした)"
fi

echo ""
echo "  ✅ VPCリソースの削除が完了しました"

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "✅ クリーンアップ完了"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "すべてのリソースの削除を試みました。"
echo "Google Cloudコンソールで残存リソースがないことを確認してください。"
echo ""
