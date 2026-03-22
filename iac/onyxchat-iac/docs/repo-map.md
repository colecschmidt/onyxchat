# OnyxChat Repository Map

This document maps the OnyxChat platform across its repositories and explains how each one contributes to the overall system.

---

## Platform Overview

OnyxChat is organized as a small multi-repository platform rather than a monolith.

The main platform is made up of:

- **Frontend client**
- **Backend API and realtime messaging service**
- **Async job processing**
- **Infrastructure as Code**
- **Observability and deployment documentation**

This structure reflects the system’s architectural boundaries while keeping each repository focused.

---

## Repositories

### Frontend
**Repository:** `web`  
**URL:** `https://github.com/splitcell01/web`

Primary React / PWA client for OnyxChat.

Responsibilities:
- user interface
- authentication flow
- conversation views
- message composition
- API integration
- WebSocket connection initiation

This is the primary client interface for the platform.

---

### Backend API + WebSocket
**Repository:** `secure-messenger-server`  
**URL:** `https://github.com/splitcell01/secure-messenger-server`

Go backend for both control-plane and realtime messaging responsibilities.

Responsibilities:
- JWT authentication
- user/session handling
- conversation and message APIs
- durable message persistence
- WebSocket connection handling
- realtime message fanout
- presence / connection state

This repository contains the main application engine of the platform.

---

### Async Job Queue
**Repository:** `batchq`  
**URL:** `https://github.com/splitcell01/batchq`

Distributed job queue used for asynchronous background processing.

Responsibilities:
- retryable jobs
- delayed/background work
- notifications
- cleanup tasks
- side-effect processing outside the main request path

This repository supports non-latency-sensitive workloads and reliability-oriented workflows.

---

### Infrastructure as Code
**Repository:** `onyxchat-iac`  
**URL:** `https://github.com/splitcell01/onyxchat-iac`

Infrastructure definition and deployment baseline for the OnyxChat platform.

Responsibilities:
- cloud resource provisioning
- networking
- load balancing / ingress direction
- deployment architecture
- cost controls
- platform documentation

This repository acts as the infrastructure and architecture source of truth.

---

## Secondary / Legacy Repository

### Desktop Client (Legacy)
**Repository:** `secure-messenger-desktop`  
**URL:** `https://github.com/splitcell01/secure-messenger-desktop`

Earlier JavaFX desktop client for the OnyxChat system.

Responsibilities:
- alternate desktop interface
- JVM/client experimentation
- early product exploration

This is not the primary client path today, but it remains a useful supporting project and part of the platform’s evolution.

---

## Architectural Roles

The repositories align to the following system roles:

### Product Layer
- `web`
- `secure-messenger-server`

These repos define the main user-facing application experience.

### Runtime / Processing Layer
- `secure-messenger-server`
- `batchq`

These repos handle request processing, realtime messaging, and asynchronous work.

### Platform Layer
- `onyxchat-iac`

This repo defines how the system is provisioned, deployed, and operated.

### Supporting / Historical Layer
- `secure-messenger-desktop`

This repo reflects earlier client experimentation and broader product exploration.

---

## Data and Platform Dependencies

The repositories depend on shared platform services that are not necessarily implemented as standalone repos:

- **Managed PostgreSQL**
  - durable application storage
  - kept external to the application runtime for cleaner separation of concerns

- **Ingress / Load Balancing**
  - external traffic entrypoint
  - routing to frontend, API, and realtime services

- **Observability**
  - metrics, logs, and dashboards
  - provides operational visibility across the platform

---

## Why This Structure

This repo layout keeps the platform modular without overcomplicating it.

Benefits:
- clear separation of concerns
- easier explanation in interviews and docs
- simpler iteration on frontend, backend, infra, and async systems independently
- cleaner mental model of the platform as a real distributed system

---

## Recommended Reading Order

For someone trying to understand the system quickly:

1. `onyxchat-iac` — infrastructure and architecture
2. `secure-messenger-server` — core backend and realtime engine
3. `web` — primary client interface
4. `batchq` — async processing model
5. `secure-messenger-desktop` — legacy / secondary client