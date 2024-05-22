# Kubernetes Wrapper for LM-Evaluation-Harness (lm-eval)

This project is a Kubernetes wrapper for the `lm-evaluation-harness`, aimed at 
facilitating the deployment and management of language model evaluations within 
Kubernetes/OpenShift environments. It's currently under active development and 
resides in this repository as a work-in-progress.

## Overview

The Kubernetes wrapper for `lm-evaluation-harness` (hereafter referred to as 
`lm-eval-aas`) extends the basic functionality of the original tool by 
integrating with Kubernetes APIs for state management and deploying as a Custom 
Resource (CR). This integration allows for more scalable and flexible deployment
options suitable for various computational and storage needs.

## Architecture Decision Record (ADR)

We have an ADR available for this project, which outlines the rationale behind 
major architectural decisions. You can view and comment on the ADR here: 
[View ADR](https://ibm.ent.box.com/file/1506388662733?s=ncewoy3bp2hb5wdbyge1rmb60sds3lkx).

## Key Features (None Currently Implemented)

- **Kubernetes API Integration**: Unlike previous demonstrations that managed  
state with a NoSQL database or local disk storage, `lm-eval-aas` uses the 
Kubernetes API for state management, enhancing the robustness and scalability of
the application.

- **Deployment as a Custom Resource (CR)**: The tool is designed to be deployed 
as a CR within a Kubernetes cluster, allowing for better integration with 
existing cluster management practices and tools.

- **Support for Persistent Volume Claims (PVCs)**: `lm-eval-aas` can mount PVCs 
to access custom data sets. This is particularly useful for evaluations that 
require large or specific data sets not typically stored within the cluster, and 
for customers with sentative data handling requirements.

## Development Status

This project is currently in a work-in-progress state. Contributions and feedback are welcome. Please refer to the issue tracker in this repository to report bugs or suggest enhancements.

## Getting Started

As the project is still under development, detailed instructions on deploying 
and using `lm-eval-aas` will be provided as the features are finalized and the 
project reaches a stable release.


# Diagrams

#### Mermaid chart links:

### Flow Diagram

```mermaid
graph LR;
    A[Client] --> B{Request};
    B -->|GET| C[Kubernetes Deployment];
    C -->|Process Request| D1[Pod 1];
    C -->|Process Request| D2[Flask App];
    F --> |Response| C;
    C -->|Process Request| D3[Pod N];
    D1 -->C;
    D3 -->C;
    C -->|Response| B;
    B -->|Response| A;

    subgraph Pod2
        D2((Flask App))
        D2 --> E1{Parse Arguments};
        E1 -->|Command Line Invocation| E2[lm-eval];
        E2 --> E3[TGIS];
        E2 --> E4[BAM];
        E2 --> E5[RHOAI Inference];
        E2 --> E6[...Other];
        E3 --> |Inference| E2;
        E4 --> |Inference| E2;
        E5 --> |Inference| E2;
        E6 --> |Inference| E2;
        D2 --> F[Calculate Metrics];
        E2 --> F;
        
    end
```

### Architecture Diagram

```mermaid
graph LR;

subgraph "Docker Container" 
    %% style rounded
    A[Install lm-eval and Flask]
    B[Expose REST Interface to lm-eval cli via Flask]
    C[Run lm-eval via cli with transpiled parameters]
end;

subgraph "Kubernetes/OpenShift Deployment" 
    %% style rounded
    D[Ingress]
    E[Load Balancer]
    F[Pod 1]
    G[Pod 2]
    H[...]
end;

subgraph "Log and Output Storage" 
    %% style rounded
    I[Log Stream]
    J[output.json Storage -PVC/COS/etc.]
end;

subgraph "User" 
    %% style rounded
    K[UI]
    L[Client]
end;

A --> B;
B --> C;

D --> E;
E -->|Ticket ID|D;
E --> F;
F -->|Ticket ID|E;
E --> G;
E --> H;

F --> I;
G --> I;
H --> I;

F --> J;
G --> J;
H --> J;

K --> L;

L --> |GET| D;
D --> |Ticket ID| L;
K --> |Ticket ID| I;
K --> |Ticket ID| J;
L --> |Ticket ID| K;
I --> |Logs| K;
J --> |Results| K;
```

