# canhazgpu
An ok-ish GPU reservation tool for single host shared development systems

## Overview

```
❯ canhazgpu
Usage: canhazgpu [OPTIONS] COMMAND [ARGS]...

Options:
  --help  Show this message and exit.

Commands:
  admin
  run
  status
```

First, I initialized it to tell it how many GPUs to track:

```
❯ canhazgpu admin --gpus 8
Initialized 8 GPUs (IDs 0 to 7)
```

Now you can see 8 GPUs as available (based on its view)

```
❯ canhazgpu status
GPU 0: AVAILABLE
GPU 1: AVAILABLE
GPU 2: AVAILABLE
GPU 3: AVAILABLE
GPU 4: AVAILABLE
GPU 5: AVAILABLE
GPU 6: AVAILABLE
GPU 7: AVAILABLE
```

If I want to run vllm with a single GPU, it's like this:

```
canhazgpu run --gpus 1 -- vllm serve my/model
```

- reserves a GPU, sets `CUDA_VISIBLE_DEVICES`, runs the command, automatically releases the GPU when the command finishes

while my vllm instance is running

```
❯ canhazgpu status
GPU 0: IN USE by rbryant for 0h 0m 6s (last heartbeat 0h 0m 6s ago)
GPU 1: AVAILABLE
GPU 2: AVAILABLE
GPU 3: AVAILABLE
GPU 4: AVAILABLE
GPU 5: AVAILABLE
GPU 6: AVAILABLE
GPU 7: AVAILABLE
```

## Requirements

- Python
- `redis` Python library
- Redis db running and listening on localhost

## TODO

- [ ] Add locking to avoid race conditions during reservation
- [ ] Add manual reservation/release commands, including time for reservation to auto-expire
- [ ] Create a proper Python package
