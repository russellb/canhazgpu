# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

This is `canhazgpu`, a GPU reservation tool for single host shared development systems. It's a Python CLI tool that uses Redis as a backend to coordinate GPU allocations across multiple users and processes.

## Architecture

The tool consists of a single Python script (`canhazgpu`) that implements three main commands:
- `admin`: Initialize and configure the GPU pool
- `status`: Show current GPU allocation status 
- `run`: Reserve GPU(s) and execute a command with `CUDA_VISIBLE_DEVICES` set

### Core Components

- **Redis Integration**: Uses Redis (localhost:6379) for persistent state management with keys under `canhazgpu:` prefix
- **GPU Allocation Logic**: Tracks GPU state with JSON objects containing user, timestamps, and heartbeat data
- **Heartbeat System**: Background thread sends periodic heartbeats (60s interval) to maintain reservations
- **Auto-cleanup**: GPUs are automatically released when heartbeat expires (15 min timeout) or process terminates

## Development Commands

### Installation
```bash
make install          # Install to /usr/local/bin with bash completion
```

### Usage Examples
```bash
# Initialize GPU pool
./canhazgpu admin --gpus 8

# Check status
./canhazgpu status

# Run command with GPU reservation
./canhazgpu run --gpus 1 -- python train.py
```

## Dependencies

- Python standard library (os, sys, time, json, subprocess, threading, datetime)
- External packages: `click`, `redis`
- System requirement: Redis server running on localhost:6379

## Key Implementation Details

- GPU reservation uses first-available allocation strategy (`canhazgpu:71-89`)
- Heartbeat mechanism prevents stale reservations (`canhazgpu:92-102`)
- Command execution preserves exit codes and handles cleanup in finally block (`canhazgpu:138-144`)
- Bash completion supports passthrough to wrapped commands after `--` separator

## Redis Schema

- `canhazgpu:gpu_count`: Total number of available GPUs
- `canhazgpu:gpu:{id}`: JSON object with `user`, `start_time`, `last_heartbeat` fields (empty object = available)