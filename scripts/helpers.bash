function gsmop_install() {
    IMAGE_TAG=$(git rev-parse --short HEAD)
    make docker-buildx install deploy IMG=${REGISTRY}/gsm-operator:${IMAGE_TAG} PLATFORMS=linux/amd64
}

function gsmop_uninstall() {
    make uninstall undeploy
}