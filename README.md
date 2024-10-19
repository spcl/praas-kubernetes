# PraaS (Process-as-a-Service) on Kubernetes and Knative

This repository contains the implementation of the PraaS model on top of Kubernetes and Knative, and associated benchmarks and their results.
This work has been done as a Master's thesis by Gyorgy Rethy. When using the code, please cite the paper and thesis:

```
@misc{praasthesis,
  title		= {{Process-as-a-Service Computing on Modern Serverless Platforms}},
  howpublished	= {\url{https://www.research-collection.ethz.ch/handle/20.500.11850/599515}},
  year		= 2022,
  author = {Gyorgy Rethy},
  note		= {Master's Thesis}
}

@inproceedings{copik2024praas,
  author = {Copik, Marcin and Calotoiu, Alexandru and Rethy, Gyorgy and Böhringer, Roman and Bruno, Rodrigo and Hoefler, Torsten},
  title = {Process-as-a-Service: Unifying Elastic and Stateful Clouds with Serverless Processes},
  year = {2024},
  isbn = {9798400712869},
  publisher = {Association for Computing Machinery},
  address = {New York, NY, USA},
  url = {https://doi.org/10.1145/3698038.3698567},
  doi = {10.1145/3698038.3698567},
  booktitle = {Proceedings of the 2024 ACM Symposium on Cloud Computing},
  keywords = {Serverless, Function-as-a-Service, Operating Systems},
  location = {Redmond, WA, USA},
  series = {SoCC '24}
}
```

## Instructions

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
