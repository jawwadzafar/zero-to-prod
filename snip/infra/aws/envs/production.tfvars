environment       = "production"
api_count         = 3
db_instance_class = "db.t4g.small"
multi_az          = true # a standby database in a second zone
# Pin production to an exact, tested image (chapter 9.4):
# snip_image = "ghcr.io/jawwadzafar/snip@sha256:..."
