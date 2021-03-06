#OCM_IMAGE=registry.svc.ci.openshift.org/openshift/release:intly-golang-1.12
#OCM=docker run --rm -it -u 1000 -v "/home/mnairn/go/src/github.com/integr8ly/integreatly-operator:/integreatly-operator/" -w "/integreatly-operator" -v "${HOME}/tmp-home:/myhome:z" -e "HOME=/myhome" --entrypoint=/usr/local/bin/ocm ${OCM_IMAGE}
UNAME=$(shell uname)
OCM=ocm
OCM_CLUSTER_NAME=rhmi-$(shell date +"%y%m%d-%H%M")
# Lifespan in hours from the time the cluster.json was created
OCM_CLUSTER_LIFESPAN=4

define get_cluster_id
	@$(eval OCM_CLUSTER_ID=$(shell mkdir -p ocm && touch ocm/cluster-details.json && jq -r .id < ocm/cluster-details.json ))
endef

define get_kubeadmin_password
	@$(eval KUBEADMIN_PASSWORD=$(shell $(OCM) get /api/clusters_mgmt/v1/clusters/${OCM_CLUSTER_ID}/credentials | jq -r .admin.password ))
endef

define save_cluster_credentials
	@$(OCM) get /api/clusters_mgmt/v1/clusters/${OCM_CLUSTER_ID}/credentials | jq -r .kubeconfig > ocm/cluster.kubeconfig
	@$(OCM) get /api/clusters_mgmt/v1/clusters/${OCM_CLUSTER_ID}/credentials | jq -r .admin | tee ocm/cluster-credentials.json
endef

ifeq ($(UNAME), Linux)
	OCM_CLUSTER_EXPIRATION_TIMESTAMP=$(shell date --date="${OCM_CLUSTER_LIFESPAN} hour" "+%FT%TZ")
else ifeq ($(UNAME), Darwin)
	OCM_CLUSTER_EXPIRATION_TIMESTAMP=$(shell date -v+${OCM_CLUSTER_LIFESPAN}H "+%FT%TZ")
endif

.PHONY: ocm/version
ocm/version:
	@${OCM} version

ocm/login: export OCM_URL := https://api.stage.openshift.com/
.PHONY: ocm/login
ocm/login:
	@${OCM} login --url=$(OCM_URL) --token=$(OCM_TOKEN)

.PHONY: ocm/whoami
ocm/whoami:
	@${OCM} whoami

.PHONY: ocm/execute
ocm/execute:
	${OCM} ${CMD}

.PHONY: ocm/get/current_account
ocm/get/current_account:
	@${OCM} get /api/accounts_mgmt/v1/current_account

.PHONY: ocm/cluster/list
ocm/cluster/list:
	@${OCM} cluster list

.PHONY: ocm/cluster/create
ocm/cluster/create: ocm/cluster/send_create_request
	@$(call get_cluster_id)
	$(call wait_command, $(OCM) get /api/clusters_mgmt/v1/clusters/${OCM_CLUSTER_ID}/status | jq -r .state | grep -q ready, cluster creation, 120m, 300)
	$(call wait_command, $(OCM) get /api/clusters_mgmt/v1/clusters/${OCM_CLUSTER_ID}/credentials | jq -r .admin | grep -q admin, fetching cluster credentials, 10m, 30)
	@echo "Console URL:"
	@$(OCM) get /api/clusters_mgmt/v1/clusters/${OCM_CLUSTER_ID} | jq -r .console.url
	@echo "Login credentials:"
	@$(call save_cluster_credentials)

.PHONY: ocm/cluster/send_create_request
ocm/cluster/send_create_request:
	@${OCM} post /api/clusters_mgmt/v1/clusters --body=ocm/cluster.json | jq -r > ocm/cluster-details.json

.PHONY: ocm/install/rhmi-addon
ocm/install/rhmi-addon:
	@$(call get_cluster_id)
	@echo '{"addon":{"id":"rhmi"}}' | ${OCM} post /api/clusters_mgmt/v1/clusters/${OCM_CLUSTER_ID}/addons
	$(call wait_command, oc --config=ocm/cluster.kubeconfig get installation -n redhat-rhmi-operator | grep -q integreatly, installation CR created, 10m, 30)
	$(call wait_command, oc --config=ocm/cluster.kubeconfig get installation integreatly -n redhat-rhmi-operator -o json | jq -r .status.stages.\\\"solution-explorer\\\".phase | grep -q completed, rhmi installation, 60m, 300)
	@oc --config=ocm/cluster.kubeconfig get installation integreatly -n redhat-rhmi-operator -o json | jq -r '.status.stages'

.PHONY: ocm/cluster/delete
ocm/cluster/delete:
	@$(call get_cluster_id)
	${OCM} delete /api/clusters_mgmt/v1/clusters/$(OCM_CLUSTER_ID)

.PHONY: ocm/cluster.json
ocm/cluster.json: export OCM_CLUSTER_REGION := "eu-west-1"
ocm/cluster.json:
	@mkdir -p ocm
	sed "s/OCM_CLUSTER_NAME/$(OCM_CLUSTER_NAME)/g" templates/ocm-cluster/cluster-template.json | \
	sed "s/OCM_CLUSTER_REGION/$(OCM_CLUSTER_REGION)/g" | \
	sed "s/OCM_CLUSTER_EXPIRATION_TIMESTAMP/$(OCM_CLUSTER_EXPIRATION_TIMESTAMP)/g" > ocm/cluster.json
