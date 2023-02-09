package framework

func CreateCluster(cluster, region, k8s_version string) error {
	return RunScript("create_cluster.sh", ApiToken, cluster, Image, k8s_version, region)
}

func DeleteCluster(clusterName string) error {
	return RunScript("delete_cluster.sh", clusterName)
}
