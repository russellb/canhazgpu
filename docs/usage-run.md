# Running Commands with GPU Reservation

The `run` command is the most common way to use canhazgpu. It reserves GPUs, runs your command with proper environment setup, and automatically cleans up when done.

## Basic Usage

```bash
canhazgpu run --gpus <count> -- <command>
```

The `--` separator is important - it tells canhazgpu where its options end and your command begins.

!!! tip "Bash Completion"
    For tab completion to work with commands after `--`, make sure you have installed the canhazgpu bash completion script. See the [Installation Guide](installation.md#bash-completion) for details.

## Common Examples

### Single GPU Training
```bash
# Reserve 1 GPU for a PyTorch training script
canhazgpu run --gpus 1 -- python train.py

# With additional arguments
canhazgpu run --gpus 1 -- python train.py --batch-size 32 --epochs 100 --lr 0.001
```

### Multi-GPU Training
```bash
# PyTorch Distributed Training
canhazgpu run --gpus 2 -- python -m torch.distributed.launch --nproc_per_node=2 train.py

# Horovod training
canhazgpu run --gpus 4 -- horovodrun -np 4 python train.py

# Custom multi-GPU script
canhazgpu run --gpus 2 -- python multi_gpu_train.py --world-size 2
```

### Inference and Serving
```bash
# vLLM model serving
canhazgpu run --gpus 1 -- vllm serve microsoft/DialoGPT-medium --port 8000

# TensorRT inference
canhazgpu run --gpus 1 -- python inference.py --model model.trt

# Jupyter notebook server
canhazgpu run --gpus 1 -- jupyter notebook --ip=0.0.0.0 --port=8888
```

## How It Works

When you run `canhazgpu run --gpus 2 -- python train.py`, here's what happens:

1. **GPU Validation**: Uses nvidia-smi to check actual GPU usage
2. **Conflict Detection**: Identifies GPUs in use without proper reservations
3. **Allocation**: Reserves 2 GPUs using LRU (Least Recently Used) strategy
4. **Environment Setup**: Sets `CUDA_VISIBLE_DEVICES` to the allocated GPU IDs (e.g., "0,3")
5. **Command Execution**: Runs `python train.py` with the GPU environment
6. **Heartbeat**: Maintains reservation with periodic heartbeats while running
7. **Cleanup**: Automatically releases GPUs when the command exits

## Environment Variables

The `run` command automatically sets:

- `CUDA_VISIBLE_DEVICES`: Comma-separated list of allocated GPU IDs
- Your command sees only the reserved GPUs as GPU 0, 1, 2, etc.

Example: If GPUs 1 and 3 are allocated, `CUDA_VISIBLE_DEVICES=1,3` is set, and your PyTorch code will see them as `cuda:0` and `cuda:1`.

## Advanced Usage

### Long-Running Commands
```bash
# Training that might take days
canhazgpu run --gpus 4 -- python long_training.py --checkpoint-every 1000

# Background processing
nohup canhazgpu run --gpus 1 -- python process_data.py > output.log 2>&1 &
```

### Complex Commands
```bash
# Multiple commands in sequence
canhazgpu run --gpus 1 -- bash -c "python preprocess.py && python train.py && python evaluate.py"

# Commands with pipes and redirects
canhazgpu run --gpus 1 -- bash -c "python train.py 2>&1 | tee training.log"
```

### Resource-Intensive Applications
```bash
# High-memory workloads
canhazgpu run --gpus 2 -- python large_model_training.py --model-size xl

# Distributed computing frameworks
canhazgpu run --gpus 4 -- dask-worker --nthreads 1 --memory-limit 8GB
```

## Error Handling

### Insufficient GPUs
```bash
❯ canhazgpu run --gpus 3 -- python train.py
Error: Not enough GPUs available. Requested: 3, Available: 2 (1 GPUs in use without reservation - run 'canhazgpu status' for details)
```

When this happens:
1. Check `canhazgpu status` to see current allocations
2. Wait for other jobs to complete
3. Contact users with unreserved GPU usage
4. Reduce the number of requested GPUs

### Command Failures
```bash
❯ canhazgpu run --gpus 1 -- python nonexistent.py
Error: Command failed with exit code 1
```

If your command fails:
- GPUs are still automatically released
- Check the command syntax and file paths
- Verify your Python environment and dependencies

### Allocation Failures
```bash
❯ canhazgpu run --gpus 1 -- python train.py
Error: Failed to acquire allocation lock after 5 attempts
```

This indicates high contention. Try again in a few seconds.

## Best Practices

### Resource Planning
- **Estimate GPU needs**: Start with fewer GPUs and scale up if needed
- **Monitor memory usage**: Use `nvidia-smi` during training to optimize allocation
- **Test with small datasets**: Verify your code works before requesting many GPUs

### Command Structure
- **Use absolute paths**: Avoid relative paths that might not work in different contexts
- **Handle signals properly**: Ensure your code can be interrupted gracefully
- **Save checkpoints frequently**: In case of unexpected termination

### Error Recovery
```bash
# Save intermediate results
canhazgpu run --gpus 2 -- python train.py --save-every 100 --resume-from checkpoint.pth

# Implement timeout handling
canhazgpu run --gpus 1 -- timeout 3600 python train.py  # 1 hour timeout
```

## Integration with Job Schedulers

### SLURM Integration
```bash
#!/bin/bash
#SBATCH --job-name=gpu_training
#SBATCH --nodes=1
#SBATCH --ntasks=1

# Use canhazgpu within SLURM job
canhazgpu run --gpus 2 -- python train.py
```

### Systemd Services
```ini
[Unit]
Description=GPU Training Service
After=network.target

[Service]
Type=simple
User=researcher
WorkingDirectory=/home/researcher/project
ExecStart=/usr/local/bin/canhazgpu run --gpus 1 -- python service.py
Restart=always

[Install]
WantedBy=multi-user.target
```

## Monitoring and Debugging

### Resource Usage
```bash
# Monitor GPU usage while job runs
watch -n 5 nvidia-smi

# Check heartbeat status
canhazgpu status  # Look for "last heartbeat" info
```

### Log Analysis
```bash
# Capture all output
canhazgpu run --gpus 1 -- python train.py 2>&1 | tee full_log.txt

# Monitor log in real-time
canhazgpu run --gpus 1 -- python train.py 2>&1 | tee training.log &
tail -f training.log
```

The `run` command provides a robust, automatic way to manage GPU reservations for your workloads while ensuring fair resource sharing across your team.