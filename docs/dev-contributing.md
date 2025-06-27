# Contributing

Thank you for your interest in contributing to canhazgpu! This guide covers how to contribute code, report issues, and help improve the project.

## Getting Started

### 1. Development Environment Setup

**Clone the repository:**
```bash
git clone https://github.com/russellb/canhazgpu.git
cd canhazgpu
```

**Install dependencies:**
```bash
# System requirements
# - Redis server
# - Python 3.6+
# - NVIDIA drivers with nvidia-smi

# Python dependencies
pip install redis click

# Development dependencies
pip install pytest pytest-mock black flake8 mypy

# Documentation dependencies (optional)
pip install -r requirements-docs.txt
```

**Set up test environment:**
```bash
# Start Redis for testing
redis-server --daemonize yes

# Verify environment
python3 canhazgpu --help
```

### 2. Development Workflow

**Create a feature branch:**
```bash
git checkout -b feature/your-feature-name
```

**Make your changes:**
- Edit the `canhazgpu` script
- Add tests for new functionality
- Update documentation if needed

**Run tests:**
```bash
# Unit tests
pytest tests/

# Integration tests
pytest tests/integration/

# Linting
flake8 canhazgpu
black --check canhazgpu
mypy canhazgpu
```

**Commit your changes:**
```bash
git add .
git commit -m "feat: add your feature description"
```

### 3. Documentation Development

**Install documentation dependencies:**
```bash
make docs-deps
# or manually: pip install -r requirements-docs.txt
```

**Build and preview documentation:**
```bash
# Build documentation
make docs

# Preview documentation locally
make docs-preview
# Opens http://127.0.0.1:8000 in your browser

# Clean build files
make docs-clean
```

## Code Style and Standards

### 1. Python Style Guide

Follow PEP 8 with these specific guidelines:

**Line length:** 88 characters (Black formatter default)
**Imports:** Group standard library, third-party, and local imports
**Docstrings:** Use Google-style docstrings
**Type hints:** Add type hints for new functions

**Example:**
```python
def reserve_gpus(
    gpu_count: int, 
    duration: str, 
    user: Optional[str] = None
) -> List[int]:
    """Reserve GPUs for specified duration.
    
    Args:
        gpu_count: Number of GPUs to reserve
        duration: Duration in format '2h', '30m', '1d'
        user: Username (defaults to current user)
        
    Returns:
        List of allocated GPU IDs
        
    Raises:
        AllocationError: If insufficient GPUs available
        ValueError: If duration format is invalid
    """
    # Implementation
```

### 2. Code Formatting

Use Black for automatic formatting:
```bash
# Format code
black canhazgpu

# Check formatting
black --check canhazgpu
```

### 3. Linting

Use flake8 for style checking:
```bash
flake8 canhazgpu --max-line-length=88 --extend-ignore=E203,W503
```

### 4. Type Checking

Use mypy for type checking:
```bash
mypy canhazgpu --ignore-missing-imports
```

## Testing Guidelines

### 1. Test Structure

**Test organization:**
```
tests/
├── unit/
│   ├── test_allocation.py
│   ├── test_validation.py
│   └── test_state_management.py
├── integration/
│   ├── test_redis_integration.py
│   └── test_nvidia_integration.py
└── fixtures/
    ├── redis_mock.py
    └── nvidia_mock.py
```

### 2. Unit Testing

**Mock external dependencies:**
```python
import pytest
from unittest.mock import patch, MagicMock

@patch('subprocess.run')
@patch('redis.Redis')
def test_detect_gpu_usage(mock_redis, mock_subprocess):
    """Test GPU usage detection with mocked nvidia-smi."""
    # Mock nvidia-smi output
    mock_subprocess.return_value.stdout = "1024\n512\n256\n"
    mock_subprocess.return_value.returncode = 0
    
    # Test the function
    usage = detect_gpu_usage()
    
    # Assertions
    assert len(usage) == 3
    assert usage[0]['memory_mb'] == 1024
```

**Test error conditions:**
```python
def test_allocation_insufficient_gpus():
    """Test allocation with insufficient GPUs."""
    with pytest.raises(AllocationError) as exc_info:
        reserve_gpus(gpu_count=10, duration="1h")
    
    assert "Not enough GPUs available" in str(exc_info.value)
```

### 3. Integration Testing

**Redis integration tests:**
```python
import redis
import pytest

@pytest.fixture
def redis_client():
    """Provide clean Redis instance for testing."""
    client = redis.Redis(host='localhost', port=6379, db=15)  # Test DB
    client.flushdb()  # Clean state
    yield client
    client.flushdb()  # Cleanup

def test_gpu_state_persistence(redis_client):
    """Test GPU state persistence in Redis."""
    # Set up initial state
    initialize_gpu_pool(8, redis_client)
    
    # Reserve a GPU
    allocated = reserve_gpus(1, "1h", redis_client)
    
    # Verify state persistence
    gpu_state = redis_client.get(f"canhazgpu:gpu:{allocated[0]}")
    assert gpu_state is not None
```

### 4. Test Coverage

Aim for >90% test coverage:
```bash
# Install coverage tool
pip install coverage

# Run tests with coverage
coverage run -m pytest tests/
coverage report
coverage html  # Generate HTML report
```

## Feature Development

### 1. Adding New Commands

To add a new command:

1. **Add Click command:**
```python
@main.command()
@click.option('--option', help='Option description')
def new_command(option):
    """Command description."""
    try:
        result = implement_new_command(option)
        click.echo(f"Success: {result}")
    except Exception as e:
        click.echo(f"Error: {e}", err=True)
        sys.exit(1)
```

2. **Implement core logic:**
```python
def implement_new_command(option: str) -> str:
    """Implement the new command functionality.
    
    Args:
        option: Command option value
        
    Returns:
        Success message
        
    Raises:
        CommandError: If command fails
    """
    # Implementation
```

3. **Add tests:**
```python
def test_new_command():
    """Test new command functionality."""
    result = implement_new_command("test_value")
    assert result == "expected_result"
```

4. **Update documentation:**
- Add to `docs/commands.md`
- Update help text
- Add usage examples

### 2. Adding New Allocation Strategies

To add alternative allocation strategies:

1. **Create strategy function:**
```python
def get_available_gpus_thermal_aware(
    gpu_count: int, 
    requested: int,
    redis_client: redis.Redis
) -> List[int]:
    """Allocate GPUs based on thermal conditions.
    
    Args:
        gpu_count: Total number of GPUs
        requested: Number of GPUs requested
        redis_client: Redis connection
        
    Returns:
        List of GPU IDs sorted by thermal preference
    """
    # Query GPU temperatures
    temperatures = get_gpu_temperatures()
    
    # Sort by coolest first
    available = get_available_gpus(gpu_count, redis_client)
    return sorted(available, key=lambda gpu_id: temperatures[gpu_id])[:requested]
```

2. **Integrate with allocation logic:**
```python
# Add strategy selection parameter
def atomic_reserve_gpus(
    requested_gpus: int,
    user: str,
    reservation_type: str,
    strategy: str = "lru",
    expiry_time: Optional[float] = None
) -> List[int]:
    """Reserve GPUs using specified allocation strategy."""
    
    if strategy == "lru":
        available = get_available_gpus_sorted_by_lru(...)
    elif strategy == "thermal":
        available = get_available_gpus_thermal_aware(...)
    else:
        raise ValueError(f"Unknown strategy: {strategy}")
```

### 3. Adding New Validation Sources

To add support for AMD GPUs or other hardware:

1. **Create validation module:**
```python
def detect_amd_gpu_usage() -> Dict[int, Dict]:
    """Detect AMD GPU usage via rocm-smi."""
    try:
        result = subprocess.run([
            'rocm-smi', '--showuse', '--csv'
        ], capture_output=True, text=True, check=True)
        
        return parse_amd_usage(result.stdout)
    except (subprocess.CalledProcessError, FileNotFoundError):
        return {}
```

2. **Integrate with main validation:**
```python
def detect_gpu_usage() -> Dict[int, Dict]:
    """Detect GPU usage from all available sources."""
    usage = {}
    
    # NVIDIA GPUs
    nvidia_usage = detect_nvidia_gpu_usage()
    usage.update(nvidia_usage)
    
    # AMD GPUs  
    amd_usage = detect_amd_gpu_usage()
    usage.update(amd_usage)
    
    return usage
```

## Documentation

### 1. Code Documentation

**Docstring requirements:**
- All public functions must have docstrings
- Include parameter descriptions and types
- Document return values and exceptions
- Provide usage examples for complex functions

**Inline comments:**
- Explain complex logic
- Document assumptions and edge cases
- Reference external resources (RFCs, papers, etc.)

### 2. User Documentation

When adding features, update:
- `docs/commands.md` - Command reference
- `docs/usage-*.md` - Usage examples
- `docs/features-*.md` - Feature explanations
- `README.md` - If major feature

### 3. API Documentation

For internal APIs, document:
- Function contracts and invariants
- Error conditions and handling
- Performance characteristics
- Thread safety considerations

## Pull Request Process

### 1. Before Submitting

**Checklist:**
- [ ] Tests pass locally
- [ ] Code follows style guidelines
- [ ] Documentation updated
- [ ] Commit messages follow convention
- [ ] No merge conflicts with main branch

**Self-review:**
- Review your own changes first
- Check for debugging code or TODOs
- Verify all edge cases are handled
- Ensure error messages are helpful

### 2. Pull Request Format

**Title:** Use conventional commit format
```
feat: add thermal-aware GPU allocation
fix: handle Redis connection timeout gracefully  
docs: update installation instructions
```

**Description template:**
```markdown
## Summary
Brief description of the change and why it's needed.

## Changes
- Specific change 1
- Specific change 2
- Specific change 3

## Testing
- [ ] Unit tests added/updated
- [ ] Integration tests pass
- [ ] Manual testing performed

## Breaking Changes
None / List any breaking changes

## Related Issues
Fixes #123
Relates to #456
```

### 3. Review Process

**What reviewers look for:**
- Code correctness and logic
- Test coverage and quality
- Performance implications
- Security considerations
- Documentation completeness
- Style and maintainability

**Addressing feedback:**
- Respond to all review comments
- Make requested changes promptly
- Ask questions if feedback is unclear
- Re-request review after changes

## Issue Reporting

### 1. Bug Reports

**Include in bug reports:**
- canhazgpu version
- Operating system and version
- Python version
- Redis version
- NVIDIA driver version
- Complete error messages
- Steps to reproduce
- Expected vs actual behavior

**Bug report template:**
```markdown
## Bug Description
Clear description of the bug

## Environment
- OS: Ubuntu 20.04
- Python: 3.8.10
- Redis: 6.0.16
- NVIDIA Driver: 470.129.06
- canhazgpu version: 1.0.0

## Steps to Reproduce
1. Step 1
2. Step 2
3. Step 3

## Expected Behavior
What should happen

## Actual Behavior
What actually happens

## Error Messages
```
Complete error output
```

## Additional Context
Any other relevant information
```

### 2. Feature Requests

**Include in feature requests:**
- Clear use case description
- Proposed solution (if any)
- Alternatives considered
- Impact on existing functionality
- Example usage

## Release Process

### 1. Version Numbering

Follow semantic versioning (SemVer):
- **Major** (X.0.0): Breaking changes
- **Minor** (0.X.0): New features, backward compatible
- **Patch** (0.0.X): Bug fixes, backward compatible

### 2. Release Checklist

- [ ] All tests pass
- [ ] Documentation updated
- [ ] Version number bumped
- [ ] Changelog updated
- [ ] Performance regression tests
- [ ] Security review (if applicable)

## Community Guidelines

### 1. Code of Conduct

- Be respectful and inclusive
- Focus on constructive feedback
- Help newcomers learn and contribute
- Assume good intentions

### 2. Communication

- Use GitHub issues for bug reports and feature requests
- Use pull requests for code changes
- Tag maintainers for urgent issues
- Be patient with response times

### 3. Recognition

Contributors are recognized in:
- CONTRIBUTORS.md file
- Release notes
- Documentation acknowledgments

Thank you for contributing to canhazgpu! Your efforts help make GPU resource management better for everyone.