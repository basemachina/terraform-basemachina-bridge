# ========================================
# VPC Network
# ========================================
# Cloud RunとCloud SQL用のVPCネットワーク

resource "google_compute_network" "main" {
  name                    = "${var.service_name}-vpc"
  auto_create_subnetworks = false
  project                 = var.project_id
}

# ========================================
# Subnet
# ========================================
# Cloud Run用のサブネット（Direct VPC Egress）

# サンプル用のためVPCフローログとPrivate Google Accessの設定は省略
# （フローログのチェックIDはtrivyのバージョンにより異なるため新旧両方を指定）
#trivy:ignore:AVD-GCP-0029
#trivy:ignore:GCP-0076
#trivy:ignore:GCP-0075
resource "google_compute_subnetwork" "main" {
  name          = "${var.service_name}-subnet"
  ip_cidr_range = "10.0.0.0/24"
  region        = var.region
  network       = google_compute_network.main.id
  project       = var.project_id

  # 注意: Direct VPC Egressを使用する場合、purposeは設定しません
  # purpose = "PRIVATE" はVPC Connector専用サブネットにのみ使用されます
}

# ========================================
# Private Service Connection
# ========================================
# Cloud SQLプライベート接続用のIPアドレス範囲

resource "google_compute_global_address" "private_ip_address" {
  name          = "${var.service_name}-private-ip"
  purpose       = "VPC_PEERING"
  address_type  = "INTERNAL"
  prefix_length = 16
  network       = google_compute_network.main.id
  project       = var.project_id

  # ライフサイクル設定
  # VPC Peering接続が削除されるまで、このアドレスを削除しない
  lifecycle {
    create_before_destroy = false
  }
}

# ========================================
# VPC Peering Connection
# ========================================
# Cloud SQL用のVPCピアリング

resource "google_service_networking_connection" "private_vpc_connection" {
  network                 = google_compute_network.main.id
  service                 = "servicenetworking.googleapis.com"
  reserved_peering_ranges = [google_compute_global_address.private_ip_address.name]

  # 削除時にVPC Peering接続を放棄する
  # VPCネットワーク全体が削除されると、VPC Peeringも自動的に削除される
  deletion_policy = "ABANDON"

  # ライフサイクル設定
  lifecycle {
    # このリソースの削除を防ぐ
    # VPCネットワーク削除時に自動的にクリーンアップされる
    prevent_destroy = false
  }
}

# ========================================
# VPC Peering 強制削除用 Null Resource
# ========================================
# terraform destroy時にVPC Peeringを強制削除するためのリソース

resource "null_resource" "cleanup_vpc_peering" {
  # VPC Peeringに依存させる
  depends_on = [google_service_networking_connection.private_vpc_connection]

  # destroy時に実行
  provisioner "local-exec" {
    when    = destroy
    command = <<-EOT
      # VPC Peering削除を試みる（エラーが出ても続行）
      gcloud services vpc-peerings delete \
        --service=servicenetworking.googleapis.com \
        --network=${self.triggers.network_name} \
        --project=${self.triggers.project_id} \
        --quiet || true
    EOT
  }

  # terraform destroy時に必要な情報をトリガーとして保存
  triggers = {
    network_name = google_compute_network.main.name
    project_id   = var.project_id
  }
}
