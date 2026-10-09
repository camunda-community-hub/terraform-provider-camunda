# Cluster clients are imported by "<cluster_id>/<zeebe_client_id>".
# The client secret is only available when the client is created, so it is empty after an import.
terraform import camunda_cluster_client.test '<cluster_id>/<zeebe_client_id>'
