# Release Command

The `release` command allows you to manually release GPU reservations held by the current user.

## Overview

```bash
canhazgpu release [--gpu-ids <ids>]
```

By default, releases all manually reserved GPUs. You can optionally specify which GPU(s) to release using the `--gpu-ids` flag.

## Options

- `-G, --gpu-ids`: Specific GPU IDs to release (comma-separated, e.g., 1,3,5)

## Reservation Types

The release command can release:
- **Manual reservations** made with the `reserve` command
- **Run-type reservations** made with the `run` command (useful for cleaning up after known failures faster than waiting for heartbeat timeout)

## Examples

### Release All Manual Reservations

```bash
❯ canhazgpu release
Released 2 GPU(s): [1, 3]
```

### Release Specific GPUs

```bash
❯ canhazgpu release --gpu-ids 1,3
Released 2 GPU(s): [1, 3]
```

### No Reservations Found

```bash
❯ canhazgpu release
No manually reserved GPUs found for current user

❯ canhazgpu release --gpu-ids 0,2
No reservations found for current user on GPU(s): [0, 2]
```

## Use Cases

### 1. Clean Up After Manual Reservations

When you're done with a manual reservation before its expiry time:

```bash
# Reserve GPUs for 4 hours
❯ canhazgpu reserve --gpus 2 --duration 4h

# ... work for 2 hours ...

# Release early when done
❯ canhazgpu release
```

### 2. Clean Up After Failed Run Commands

If a `run` command process crashes or hangs and you need to clean up immediately:

```bash
# Check status to see stuck reservations
❯ canhazgpu status

# Release specific stuck GPUs
❯ canhazgpu release --gpu-ids 2,3
```

### 3. Selective Release

When you have multiple reservations but only want to release some:

```bash
# Reserve specific GPUs
❯ canhazgpu reserve --gpu-ids 0,1,2,3 --duration 2h

# Release only GPUs 0 and 1
❯ canhazgpu release --gpu-ids 0,1
Released 2 GPU(s): [0, 1]

# GPUs 2 and 3 remain reserved
```

## Important Notes

!!! note "Ownership"
    You can only release GPUs that are reserved by your user account. Attempting to release GPUs reserved by other users will have no effect.

!!! info "Run-type Reservations"
    While run-type reservations are automatically cleaned up when the process ends or after heartbeat timeout, the `--gpu-ids` option allows immediate cleanup, which is useful when you know a process has failed.

!!! tip "Best Practice"
    Always release manual reservations when you're done to free up resources for other users. The system will eventually clean them up at expiry time, but immediate release is more considerate.

## Related Commands

- [`reserve`](usage-reserve.md) - Manually reserve GPUs
- [`run`](usage-run.md) - Reserve GPUs and run a command
- [`status`](usage-status.md) - Check current GPU reservations