package framework

func CreateCluster(cluster string) error {
	return RunScript("create_cluster.sh", ApiToken, cluster, Image)
}

func DeleteCluster(clusterName string) error {
	return RunScript("delete_cluster.sh", clusterName)
}
