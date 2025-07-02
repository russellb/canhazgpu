# GitHub Pages Deployment

This guide covers deploying the canhazgpu documentation to GitHub Pages using MkDocs Material.

## Setup GitHub Pages

### 1. Repository Configuration

**Enable GitHub Pages:**

!!! important "Required Setup Steps"
    You must configure GitHub Pages in your repository settings before the workflow can deploy.

1. **Go to Repository Settings**
   - Navigate to your repository on GitHub
   - Click the "Settings" tab (you need admin/write access)

2. **Configure Pages**
   - Scroll down to "Pages" in the left sidebar
   - Under "Source", select "GitHub Actions"
   - Click "Save"

3. **Verify Configuration**
   - You should see "Your site is ready to be published at http://blog.russellbryant.net/canhazgpu/"
   - The workflow will automatically deploy when you push to main branch

**Alternative: Manual Enablement**

If you get "Pages site failed" errors, the workflow includes automatic enablement. However, manual setup is more reliable:

```bash
# Using GitHub CLI (if you have it installed)
gh api repos/russellb/canhazgpu/pages -X POST -f source=actions
```

### 2. GitHub Actions Workflow

The `.github/workflows/docs.yml` file is already configured to:

- **Build documentation** using MkDocs Material
- **Install dependencies** from `requirements-docs.txt`
- **Deploy automatically** to GitHub Pages on pushes to main branch
- **Use proper permissions** for GitHub Pages deployment
- **Support concurrent deployments** safely

Key features of the workflow:
- Only builds on documentation changes (docs/, mkdocs.yml, workflow file)
- Uses the modern GitHub Pages deployment action
- Separates build and deploy jobs for better error handling
- Includes proper permissions for Pages deployment

### 3. Local Development

**Install MkDocs Material:**
```bash
# Using requirements file (recommended)
pip install -r requirements-docs.txt

# Or manually
pip install mkdocs-material
pip install mkdocs-git-revision-date-localized-plugin
```

**Development commands:**
```bash
# Using Makefile (recommended)
make docs-preview    # Serve documentation locally
make docs           # Build documentation
make docs-clean     # Clean build files

# Or use MkDocs directly
mkdocs serve        # Serve documentation locally
mkdocs build        # Build documentation
mkdocs gh-deploy    # Deploy to GitHub Pages (if configured)
```

## MkDocs Configuration

The `mkdocs.yml` file is already configured with:

### 1. Theme Configuration
- Material Design theme
- Dark/light mode toggle
- Navigation features
- Search functionality
- Code syntax highlighting

### 2. Plugin Configuration
- **Search**: Full-text search across documentation
- **Git revision dates**: Automatic page timestamps

### 3. Markdown Extensions
- **Code highlighting**: Syntax highlighting for code blocks
- **Admonitions**: Warning, info, and note callouts
- **Tabbed content**: Organize content in tabs
- **Task lists**: Checkbox lists for TODOs

## Content Organization

### 1. Navigation Structure

The documentation is organized into logical sections:

```yaml
nav:
  - Home: index.md
  - Getting Started:
    - Installation: installation.md
    - Quick Start: quickstart.md
  - Usage:
    - Commands Overview: commands.md
    - Running Jobs: usage-run.md
    - Manual Reservations: usage-reserve.md
    - Status Monitoring: usage-status.md
  - Features:
    - GPU Validation: features-validation.md
    - Unauthorized Usage Detection: features-unauthorized.md
    - LRU Allocation: features-lru.md
  - Administration:
    - Setup & Configuration: installation.md
    - Troubleshooting: admin-troubleshooting.md
  - Development:
    - Architecture: dev-architecture.md
    - Contributing: dev-contributing.md
```

### 2. Content Guidelines

**Use admonitions for important information:**
```markdown
!!! info "Installation Note"
    Make sure Redis is running before initializing the GPU pool.

!!! warning "Breaking Change"
    Using `--force` will clear all existing reservations.

!!! tip "Performance Tip"
    Use `canhazgpu status` to check availability before large allocations.
```

**Code blocks with syntax highlighting:**
```markdown
```bash
# Command examples
canhazgpu run --gpus 2 -- python train.py
```

```python
# Python code examples
import subprocess
result = subprocess.run(['canhazgpu', 'status'])
```
```

**Cross-references between pages:**
```markdown
See the [Installation Guide](installation.md) for setup instructions.
For advanced usage, refer to [Manual Reservations](usage-reserve.md).
```

## Deployment Process

### 1. Automatic Deployment

When you push changes to the `main` branch:

1. **GitHub Actions triggers** the workflow
2. **Dependencies are installed** (MkDocs Material, plugins)
3. **Documentation is built** from markdown files
4. **Site is deployed** to `gh-pages` branch
5. **GitHub Pages serves** the updated documentation

### 2. Manual Deployment

For immediate deployment:

```bash
# Build and deploy in one command
mkdocs gh-deploy

# Or build first, then deploy
mkdocs build
git add site/
git commit -m "Update documentation"
git push origin gh-pages
```

### 3. Preview Changes

**Local preview:**
```bash
# Start development server
mkdocs serve

# Open browser to http://127.0.0.1:8000
```

**Pull request previews:**
The workflow also runs on pull requests to validate the build without deploying.

## Custom Domain Configuration

### 1. Current Custom Domain Setup

The documentation is configured to use the custom domain `blog.russellbryant.net/canhazgpu/`:

**GitHub Pages Settings:**
- Custom domain configured in repository Settings → Pages → Custom domain
- Set to: `blog.russellbryant.net/canhazgpu`

**MkDocs configuration:** `mkdocs.yml`
```yaml
site_url: http://blog.russellbryant.net/canhazgpu/
```

### 2. Changing the Custom Domain

To use a different custom domain:

1. **Update GitHub Pages Settings:**
   - Go to repository Settings → Pages
   - Under "Custom domain", enter your domain
   - Save settings

2. **Update mkdocs.yml:**
```yaml
site_url: https://your-custom-domain.com
```

### 3. DNS Configuration

Configure DNS records with your domain provider:

```
Type: CNAME
Name: your-subdomain
Value: your-username.github.io
```

### 4. SSL Certificate

GitHub Pages automatically provides SSL certificates for custom domains.

## Monitoring and Analytics

### 1. Google Analytics (Optional)

Add to `mkdocs.yml`:
```yaml
extra:
  analytics:
    provider: google
    property: G-XXXXXXXXXX
```

### 2. GitHub Pages Insights

Monitor documentation usage through:
- Repository insights
- GitHub Pages analytics
- Traffic sources and popular pages

## Troubleshooting

### 1. GitHub Pages Configuration Issues

**"Pages site failed" Error:**
```
Error: Get Pages site failed. Please verify that the repository has Pages enabled
```

**Solutions:**
1. **Manual Setup** (Recommended):
   - Go to repository Settings → Pages
   - Select "GitHub Actions" as source
   - Save settings

2. **Check Repository Permissions**:
   - Ensure you have admin/write access to the repository
   - Verify the repository is not private (free GitHub accounts)

3. **Verify Workflow Permissions**:
   - Go to Settings → Actions → General
   - Under "Workflow permissions", select "Read and write permissions"
   - Save changes

**Repository Visibility Issues:**
- GitHub Pages requires a public repository for free accounts
- Private repositories need GitHub Pro/Team/Enterprise

### 2. Build Failures

**Common issues:**
- Missing dependencies in workflow
- Markdown syntax errors
- Broken internal links
- Plugin configuration errors

**Debug locally:**
```bash
# Check for build errors
mkdocs build --strict

# Validate configuration
mkdocs config

# Check for broken links
mkdocs build --strict --verbose
```

### 3. Deployment Issues

**Check GitHub Actions:**
1. Go to "Actions" tab in repository
2. Review failed workflow runs
3. Check build logs for errors

**Common solutions:**
```bash
# Clear build cache
rm -rf site/

# Rebuild documentation
mkdocs build

# Test locally first
make docs-preview
```

### 3. Custom Domain Issues

**Verify DNS configuration:**
```bash
dig docs.canhazgpu.com
nslookup docs.canhazgpu.com
```

**Check HTTPS certificate:**
- GitHub Pages may take time to provision SSL
- Ensure CNAME file is correct
- Verify DNS propagation

## Maintenance

### 1. Dependency Updates

Regularly update MkDocs and plugins:

```bash
# Update packages
pip install --upgrade mkdocs-material
pip install --upgrade mkdocs-git-revision-date-localized-plugin

# Update workflow dependencies
# Edit .github/workflows/docs.yml with latest versions
```

### 2. Content Reviews

**Regular maintenance tasks:**
- Review and update outdated information
- Fix broken links
- Improve unclear explanations
- Add new features and changes

### 3. Performance Monitoring

**Optimize for speed:**
- Compress images
- Minimize plugin usage
- Keep pages focused and concise
- Use CDN for assets (GitHub Pages provides this)

## Best Practices

### 1. Documentation Writing

- **Write for your audience**: Mix beginners and advanced users
- **Use clear headings**: Make content scannable
- **Include examples**: Show don't just tell
- **Keep it current**: Update docs with code changes

### 2. Version Control

- **Commit docs with code**: Keep documentation in sync
- **Use meaningful commits**: Clear commit messages for doc changes
- **Review changes**: Use pull requests for documentation updates

### 3. Accessibility

- **Alt text for images**: Describe visual content
- **Clear navigation**: Logical page organization
- **Readable fonts**: MkDocs Material handles this well
- **Color contrast**: Theme provides good accessibility

The GitHub Pages deployment provides a professional, searchable documentation site that automatically stays up-to-date with your codebase changes.