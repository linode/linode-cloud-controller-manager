package framework

import "fmt"

func CreateCluster(cluster string) error  {
	fmt.Printf(ApiToken, cluster, "<>")
	return RunScript("create_cluster.sh", ApiToken, cluster)
}

func DeleteCluster() error  {
	return nil
	return RunScript("delete_cluster.sh")
}