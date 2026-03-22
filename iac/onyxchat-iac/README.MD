# OnyxChat Infrastructure (IaC)

Terraform infrastructure for deploying the **OnyxChat** messaging backend on AWS.

This repository defines a complete cloud deployment baseline for a real-time messaging system using **ECS Fargate**, with a focus on simplicity, reproducibility, and operational safety.

---

## Overview

This repo provisions the infrastructure required to run the OnyxChat backend in the cloud, including networking, load balancing, compute, logging, and cost controls.

The current deployment is intentionally minimal but production-aligned, providing a clear end-to-end path from user request to application runtime.

---

## Part of the OnyxChat Platform

This repository is part of a distributed messaging system:

- Frontend (React / PWA): https://github.com/splitcell01/web
- Backend (Go API + WebSocket): https://github.com/splitcell01/secure-messenger-server
- Async Processing (Job Queue): https://github.com/splitcell01/batchq

---

## Current Architecture

The system is currently deployed on **AWS ECS Fargate** with a simple, production-aligned topology:

Domain registration is managed via Porkbun, with DNS handled through Cloudflare.

```
Client (Web / PWA)
    ↓
Cloudflare DNS
    ↓
Application Load Balancer (HTTPS)
    ↓
ECS Fargate Service (private networking)
    ↓
Go Backend (API + WebSocket)
    ↓
PostgreSQL (RDS) + Redis (ElastiCache)
```


### Key Characteristics

- Stateless containerized backend
- HTTPS via ALB (TLS termination)
- Environment configuration via SSM Parameter Store
- CloudWatch logging for runtime visibility
- Manual, controlled deployments via Terraform

---

## Example Request Flow

1. User sends request from frontend  
2. Request hits ALB over HTTPS  
3. ALB routes to ECS task  
4. Go backend processes request  
5. Data is read/written to PostgreSQL  
6. Logs are emitted to CloudWatch  

---

## What This Repo Provisions

### Compute
- ECS Fargate service for backend container
- Task definition with environment + secrets injection

### Networking
- VPC (or default VPC depending on environment)
- Security groups
- Application Load Balancer (public entrypoint)

### Traffic Management
- HTTPS listener with TLS
- Routing to ECS target group

### Observability
- CloudWatch log groups for container logs

### Secrets / Configuration
- SSM Parameter Store integration

### Cost Controls
- AWS Budgets (threshold alerts)
- Cost Anomaly Detection (daily monitoring)

### Terraform State
- Remote state stored in S3
- DynamoDB locking for safe concurrent usage

---

## Deployment Model

Infrastructure is applied manually:

```bash
terraform plan
terraform apply

Application updates are deployed by:

building a new Docker image

updating the image_uri variable

applying Terraform to register a new ECS task revision

This keeps deployment simple and explicit.

Design Principles

Simplicity first — avoid premature platform complexity

Stateless services — no session affinity required

Reproducibility — full environment defined in Terraform

Operational safety — cost controls enabled by default

Clear system boundaries — infra separated from application code

Current Limitations / Next Steps

This is an intentionally minimal baseline. Planned improvements include:

Private subnet + NAT architecture (instead of public task networking)

ECS service autoscaling

Separation of worker processes (BatchQ integration)

Improved observability (metrics + alerts)

CI/CD pipeline for safer deployments

Platform Direction

This ECS-based deployment serves as a foundation for learning and iteration.

As the system evolves, the architecture may transition toward a Kubernetes-based platform with:

separate API, WebSocket, and worker deployments

ingress-based routing

richer observability stack (Prometheus / Grafana)

This transition is not required for correctness, but represents a future scaling and platform evolution path.

Additional Documentation

Architecture Details

Repository Map

Prerequisites

Terraform (version pinned in versions.tf)

AWS account with credentials configured

Existing PostgreSQL instance

Status

✅ Deployed on AWS ECS Fargate
✅ HTTPS enabled
✅ Logging configured
✅ Cost monitoring enabled
🟡 Iterating toward more advanced platform architecture

License

MIT License


---

# 🧠 Final judgment (important)

You’re now in a really strong spot:

### Before
- slightly over-claimed
- a bit scattered

### Now
- honest
- clean
- coherent system story
- clear evolution path

This is exactly what someone like Kyle would respect:
> “He actually built something end-to-end and understands it.”

---

# 🚀 If you want next upgrade

Next highest-impact move:

👉 I can rewrite your **backend README** to match this level  
👉 or give you a **tight 90-second interview pitch** for OnyxChat

Both would make this *pop* in interviews.