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

@inproceedings{praaspaper,
   author = {Wolfrath, Joel and Chandra, Abhishek},
   title = {Process-as-a-Service: Unifying Elastic and Stateful Clouds with Serverless Processes},
   publisher = {Association for Computing Machinery},
   address = {New York, NY, USA},
   url = {https://doi.org/10.1145/3698038.3698567},
   doi = {10.1145/3698038.3698567},
   abstract = {Fine-grained serverless functions power many new applications that benefit from elastic scaling and pay-as-you-use billing model with minimal infrastructure management overhead. To achieve these properties, Function-as-a-Service (FaaS) platforms disaggregate compute and state and, consequently, introduce non-trivial costs due to the loss of data locality when accessing state, complex control plane inter-
actions, and expensive inter-function communication. We revisit the foundations of FaaS and propose a new cloud abstraction, the cloud process, that retains all the benefits of FaaS while significantly reducing the overheads that result from disaggregation. We show how established operating system abstractions can be adapted to provide powerful granular computing on dynamically provisioned cloud resources while building our Process as a Service (PraaS) platform. PraaS improves current FaaS by offering data locality, fast invocations, and efficient communication. PraaS delivers invocations up to 32× faster and reduces communication overhead by up to 99%.},
   booktitle = {Proceedings of the 2024 ACM Symposium on Cloud Computing},
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
