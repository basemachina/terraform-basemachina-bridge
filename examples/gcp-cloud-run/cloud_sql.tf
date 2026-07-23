# ========================================
# Cloud SQL Instance
# ========================================
# PostgreSQLデータベースインスタンス（プライベートIP）

resource "random_id" "db_name_suffix" {
  byte_length = 4
}

# 接続テスト用の簡易DB（プライベートIPのみ）のため、詳細な監査ログフラグとTLS強制は省略
#trivy:ignore:AVD-GCP-0014
#trivy:ignore:AVD-GCP-0015
#trivy:ignore:AVD-GCP-0016
#trivy:ignore:AVD-GCP-0020
#trivy:ignore:AVD-GCP-0022
#trivy:ignore:AVD-GCP-0025
resource "google_sql_database_instance" "main" {
  name             = "${var.service_name}-db-${random_id.db_name_suffix.hex}"
  database_version = "POSTGRES_16"
  region           = var.region
  project          = var.project_id

  # VPCピアリングが完了するまで待機
  depends_on = [google_service_networking_connection.private_vpc_connection]

  settings {
    tier = "db-f1-micro"

    # IPアドレス設定（プライベートIPのみ）
    ip_configuration {
      ipv4_enabled    = false
      private_network = google_compute_network.main.id
    }

    # バックアップ設定
    backup_configuration {
      enabled                        = true
      point_in_time_recovery_enabled = true
      start_time                     = "03:00"
    }

    # 可用性設定
    availability_type = "ZONAL"

    # ディスク設定
    disk_type = "PD_SSD"
    disk_size = 10
  }

  deletion_protection = false

  # タイムアウト設定
  # Cloud SQLインスタンスの削除には時間がかかるため、十分な時間を確保
  timeouts {
    create = "30m"
    update = "30m"
    delete = "30m"
  }
}

# ========================================
# Cloud SQL 強制削除用 Null Resource
# ========================================
# terraform destroy時にCloud SQLを確実に削除するためのリソース

resource "null_resource" "cleanup_cloud_sql" {
  depends_on = [
    google_sql_database.database,
    google_sql_user.user
  ]

  # destroy時に実行
  provisioner "local-exec" {
    when    = destroy
    command = <<-EOT
      # Cloud SQLインスタンスを強制削除（エラーが出ても続行）
      gcloud sql instances delete ${self.triggers.instance_name} \
        --project=${self.triggers.project_id} \
        --quiet || true

      # 削除完了まで待機
      sleep 30
    EOT
  }

  triggers = {
    instance_name = google_sql_database_instance.main.name
    project_id    = var.project_id
  }
}

# ========================================
# Database
# ========================================
# アプリケーション用データベース

resource "google_sql_database" "database" {
  name     = var.database_name
  instance = google_sql_database_instance.main.name
  project  = var.project_id
}

# ========================================
# Database User
# ========================================
# データベースユーザー

resource "random_password" "db_password" {
  length  = 16
  special = true
}

resource "google_sql_user" "user" {
  name     = var.database_user
  instance = google_sql_database_instance.main.name
  password = random_password.db_password.result
  project  = var.project_id
}
