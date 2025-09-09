# Ansible Operator Plugins

A plugin that provide Ansible-based operator functionality for the [Operator SDK](https://github.com/operator-framework/operator-sdk). This project contains the core Ansible operator implementation that enables developers to build Kubernetes operators using Ansible playbooks and roles.

## Overview

This project provides the Ansible plugin for Operator SDK, allowing you to:
- Build Kubernetes operators using Ansible playbooks and roles
- Manage custom resources with Ansible automation
- Handle operator lifecycle events through Ansible tasks
- Leverage the full ecosystem of Ansible modules and collections 

# Releasing Guide

## Pre-Requisites
- Push access to this repository
- Forked repository and local clone of fork
- Remote ref named `upstream` that points to this repository

## Release Prep (Applies to all releases)
Since this project is currently consumed as a library there are some manual steps that need to take
place prior to creating a release. They are as follows:
1. Checkout the `main` branch:
```sh
git checkout main
```
2. Ensure the `main` branch is up to date:
```sh
git fetch upstream && git pull upstream main
```
3. Checkout a new branch for release prep work:
```sh
git checkout -b release/prep-vX.Y.Z
```
4. Update the `ImageVersion` variable in `internal/version/version.go` to be the version you are prepping for release
5. Update the line with `export IMAGE_VERSION` in `Makefile` to be the version you are prepping for release
6. Regenerate the testdata:
```sh
make generate
```
7. Commit and push your changes to your fork
8. Create a PR against the `main` branch

## Creating Major/Minor Releases
1. Ensure the steps in [Release Prep](#release-prep-applies-to-all-releases) have been completed. Do **NOT** progress past this point until the release prep PR has merged.
2. Checkout the `main` branch:
```sh
git checkout main
```
3. Ensure your local branch is up to date:
```sh
git fetch upstream && git pull upstream main
```
4. Checkout a new branch for the new release following the pattern `release-vX.Y`. In this example we will create a branch for a `v0.2.0` release:
```sh
git checkout -b release-v0.2
```
5. Push the newly created release branch:
```sh
git push -u upstream release-v0.2
```
6. Create a new release tag:
```sh
git tag -a -s -m "ansible-operator-plugins release v0.2.0" v0.2.0
```
7. Push the new tag:
```sh
git push upstream v0.2.0
```

## Creating Patch Releases
1. Ensure the steps in [Release Prep](#release-prep-applies-to-all-releases) have been completed. Do **NOT** progress past this point until the release prep PR has merged.
2. Cherry pick the merged release prep PR to the proper major/minor branch by commenting the following on the PR:
```
/cherry-pick release-vX.Y
```
where X is the major version and Y is the minor version. An example of cherry picking for a `v0.2.1` release would be:
```
/cherry-pick release-v0.2
```
3. A bot will have created the cherry pick PR. Merge this. Do **NOT** progress past this point until the cherry pick PR has merged.
4. Checkout the appropriate release branch. In this example we will be "creating" a `v0.2.1` release:
```sh
git checkout release-v0.2
```
5. Ensure it is up to date:
```sh
git fetch upstream && git pull upstream release-v0.2
```
6. Create a new release tag:
```sh
git tag -a -s -m "ansible-operator-plugins release v0.2.1" v0.2.1
```
7. Push the new tag:
```sh
git push upstream v0.2.1
```

> [!NOTE]
> While the release process is automated once the tag is pushed it can occasionally timeout.
> If this happens, re-running the action will re-run the release process and typically succeed.
