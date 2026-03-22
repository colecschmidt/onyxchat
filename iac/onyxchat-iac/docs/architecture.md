# OnyxChat Platform Architecture

OnyxChat is a real-time messaging platform built to explore distributed systems, realtime communication, infrastructure, and operational visibility in a single end-to-end system.

This document describes both the current ECS-based deployment and the longer-term target architecture for the OnyxChat platform.

The platform is centered around a React frontend, a Go backend, realtime WebSocket messaging, durable PostgreSQL storage, asynchronous background processing, and infrastructure defined as code.

---

## Goals

The architecture is designed to:

- separate control-plane and realtime workloads
- keep core messaging state durable
- support asynchronous background work
- make traffic flow and system boundaries easy to reason about
- provide a path from a simple cloud deployment to a fuller Kubernetes platform

---

## Current Deployment

The current cloud deployment uses **AWS ECS Fargate** as a simple, reproducible baseline for running backend services safely in the cloud.

Current infrastructure responsibilities include:

- AWS networking (VPC, subnets, security groups)
- Application Load Balancer with HTTPS
- ECS task execution for backend services
- Terraform-managed infrastructure
- cost controls and monitoring safeguards

This provides a production-safe starting point while keeping operational complexity manageable.

---

## Target Kubernetes Architecture

The target platform architecture moves the system to **Kubernetes** so the major runtime concerns can be separated into independently deployable workloads.

### Core Workloads

- **Frontend Deployment**
  - React / PWA client
  - serves the main user interface
  - handles authentication flow and API/WebSocket calls

- **API Deployment**
  - Go REST service
  - handles authentication, user management, conversations, and durable message persistence
  - acts as the control plane for the application

- **WebSocket Deployment**
  - Go realtime messaging service
  - maintains active connections
  - handles message fanout, presence, and connection state

- **BatchQ Worker Deployment**
  - asynchronous background processing
  - handles retries, notifications, cleanup tasks, and other non-user-blocking work

- **Observability Stack**
  - Prometheus for metrics
  - Grafana for dashboards
  - Loki for logs

- **PostgreSQL**
  - durable system state
  - stores users, sessions, conversations, messages, and related metadata

---

## High-Level Flow (Current Deployment)

```text
                    ┌──────────────────────┐
                    │      Frontend        │
                    │   (React / PWA)      │
                    └─────────┬────────────┘
                              │ HTTPS
                              ▼
                    ┌──────────────────────┐
                    │  AWS Route53 (DNS)   │
                    └─────────┬────────────┘
                              ▼
                    ┌──────────────────────┐
                    │ Application Load     │
                    │ Balancer (ALB)       │
                    └─────────┬────────────┘
                              ▼
                    ┌──────────────────────┐
                    │ ECS Fargate Service  │
                    │  (Go Backend)        │
                    └───────┬──────────────┘
                            │
         ┌──────────────────┼──────────────────┐
         ▼                  ▼                  ▼
┌────────────────┐  ┌────────────────┐  ┌────────────────┐
│ PostgreSQL     │  │ CloudWatch     │  │ SSM Parameter  │
│ (persistent DB)│  │ Logs           │  │ Store (secrets)│
└────────────────┘  └────────────────┘  └────────────────┘

                    (Optional / Next Step)
                           ▼
                    ┌──────────────────────┐
                    │ BatchQ Worker        │
                    │ (async processing)   │
                    └──────────────────────┘

This diagram reflects the current ECS-based deployment. Future versions of the system may separate workloads and move to a Kubernetes-based architecture.

## Target Architecture (Long-Term / Conceptual)

This architecture represents a potential future deployment of OnyxChat on a managed Kubernetes cluster (e.g. EKS or GKE), rather than a locally hosted environment.

Users
  │
  │  HTTPS / WSS
  ▼
DNS
  ├─ onyxchat.dev / app.onyxchat.dev
  └─ api.onyxchat.dev
  │
  ▼
Ingress / Load Balancer
  ├─ "/"      -> Web Frontend
  ├─ "/api"   -> Go API
  └─ "/ws"    -> WebSocket Gateway
  │
  ▼
Kubernetes Cluster
  ├─ Frontend Pod(s)
  │    └─ React / PWA
  │
  ├─ API Pod(s)
  │    ├─ Auth / JWT
  │    ├─ User & chat APIs
  │    ├─ Message persistence
  │    └─ Presence/session logic
  │
  ├─ WebSocket Pod(s)
  │    ├─ real-time connections
  │    ├─ room/session tracking
  │    └─ delivery fanout
  │
  ├─ BatchQ Worker Pod(s)
  │    ├─ notifications
  │    ├─ async delivery/retry
  │    ├─ indexing / cleanup
  │    └─ background jobs
  │
  └─ Observability Stack
       ├─ Prometheus
       ├─ Grafana
       └─ Loki
  │
  ▼
Data Layer
  ├─ PostgreSQL
  │    ├─ users
  │    ├─ conversations
  │    ├─ messages
  │    ├─ devices / sessions
  │    └─ job queue tables
  │
  └─ Object Storage (optional later)
       └─ attachments / media
