function gsmop_install() {
    TAG=$(git rev-parse --short HEAD)
    make docker-buildx install deploy IMG=${REGISTRY}/gsm-operator:${TAG} PLATFORMS=linux/amd64
}

function gsmop_uninstall() {
    make uninstall undeploy
}