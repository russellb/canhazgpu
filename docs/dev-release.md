# Release Process

This document describes how to create and publish a new release of canhazgpu.

## Prerequisites

Before creating a release, ensure you have:

- [ ] **Push access** to the main repository
- [ ] **GitHub CLI** installed and authenticated (`gh auth login`)
- [ ] **goreleaser** installed (`go install github.com/goreleaser/goreleaser@latest`)
- [ ] **Clean working directory** with all changes committed
- [ ] **Tests passing** (`make test` if available)
- [ ] **Documentation updated** for any new features

## Release Steps

### 1. Prepare the Release

```bash
# Ensure you're on the main branch with latest changes
git checkout main
git pull origin main

# Verify clean working directory
git status
# Should show "nothing to commit, working tree clean"

# Run any final tests
make test  # if test target exists
```

### 2. Determine Version Number

Follow [Semantic Versioning](https://semver.org/):

- **MAJOR** (v2.0.0): Breaking changes
- **MINOR** (v1.1.0): New features, backwards compatible
- **PATCH** (v1.0.1): Bug fixes, backwards compatible

### 3. Create and Push the Tag

```bash
# Create a new tag (replace X.Y.Z with your version)
git tag vX.Y.Z

# Push the tag to trigger the release
git push origin vX.Y.Z
```

**Example:**
```bash
git tag v1.2.3
git push origin v1.2.3
```

### 4. Build and Release with goreleaser

```bash
# Set GitHub token (get it from gh CLI)
export GITHUB_TOKEN=$(gh auth token)

# Build and release
goreleaser --clean
```

### 5. Verify the Release

After goreleaser completes:

1. **Check GitHub Releases**: Visit https://github.com/russellb/canhazgpu/releases
2. **Verify artifacts**: Ensure binaries are attached for all platforms
3. **Test installation**: Try installing from the new release
4. **Update documentation**: If installation instructions changed

## goreleaser Configuration

The release process uses `.goreleaser.yml` in the repository root. Key features:

- **Multi-platform builds**: Linux (amd64, arm64)
- **Archive creation**: Compressed binaries for each platform
- **Checksums**: SHA256 checksums for verification
- **GitHub Release**: Automatic release creation with artifacts

## Post-Release Tasks

### 1. Update Installation Documentation

If the release includes new installation methods:

```bash
# Update installation.md with new download links
# Update README.md if needed
```

### 2. Announce the Release

Consider announcing on:
- GitHub Discussions (if enabled)
- Team communication channels
- Documentation updates

### 3. Monitor for Issues

After release:
- Watch for bug reports
- Monitor installation feedback
- Be ready to create patch releases if needed

## Troubleshooting

### goreleaser Fails

**Authentication issues:**
```bash
# Check GitHub token
echo $GITHUB_TOKEN
gh auth status

# Re-authenticate if needed
gh auth login
```

**Build failures:**
```bash
# Test goreleaser config
goreleaser check

# Build without releasing (dry run)
goreleaser --snapshot --skip-publish --clean
```

### Tag Already Exists

If you need to recreate a tag:
```bash
# Delete local tag
git tag -d vX.Y.Z

# Delete remote tag (BE CAREFUL!)
git push origin :refs/tags/vX.Y.Z

# Recreate and push
git tag vX.Y.Z
git push origin vX.Y.Z
```

### Binary Issues

If released binaries have issues:

1. **Quick patch**: Create a patch release (increment patch version)
2. **Major issue**: Consider yanking the release and creating a new one
3. **Document known issues**: Update release notes

## Release Checklist

Before pushing the tag:

- [ ] Version number follows semantic versioning
- [ ] All tests pass
- [ ] Documentation is updated
- [ ] Working directory is clean
- [ ] You're on the main branch
- [ ] Recent commits are tested

After release:

- [ ] GitHub release page looks correct
- [ ] All platform binaries are present
- [ ] Installation instructions work
- [ ] Release notes are accurate
- [ ] Documentation reflects new version

## Emergency Procedures

### Hotfix Release

For critical bugs in production:

```bash
# Create hotfix branch from tag
git checkout vX.Y.Z
git checkout -b hotfix/vX.Y.Z+1

# Make minimal fix
# ... edit files ...

# Commit fix
git add .
git commit -m "fix: critical bug in X"

# Merge back to main
git checkout main
git merge hotfix/vX.Y.Z+1

# Tag and release
git tag vX.Y.Z+1
git push origin vX.Y.Z+1
GITHUB_TOKEN=$(gh auth token) goreleaser --clean
```

### Rollback

If a release has critical issues:

1. **Document the issue** in release notes
2. **Create a patch release** with fixes
3. **Consider pre-release** for testing: `git tag vX.Y.Z-rc1`

Remember: Once a release is published and people may be using it, avoid deleting releases. Instead, create new releases that fix issues.
