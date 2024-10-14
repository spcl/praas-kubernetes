To deploy to AWS:

1. In create-aws-cluster.sh uncomment: `eksctl create cluster --kubeconfig eksnative.yaml -f aws-cluster.yaml`
   1.1 This line creates the Kubernetes cluster
2. Execute create-aws-cluster.sh: Creates a kubernestes cluster and installs knative, a dns and kourier (for networking)
3. To deploy different version of praas to AWS there is the deploy-*-aws.sh scripts. Should work out of the box.

Other scripts:
 - create-cluster.sh: Creates local kind cluster for developement.
 - dev-cluster: Also creates a kind cluster. If I remember correctly this deployed the modified knative.
 - prod-cluster: Also creates a kind cluster. I think this deploys the production version of knative.
 - deploy-*.sh: These should deploy to the local kind cluster.
 - The rest are "internal" per component scripts. They expect the right kubernetes config to make kubectl work and do more fine-grained steps.


The various .yaml files are configurations for kubernetes.
