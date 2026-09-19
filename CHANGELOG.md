# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Initial OSS release

- Declarative `ZenCleanerPolicy` CRD (cleaner.zen-mesh.io/v1alpha1) with TTL modes
  (fixed, field-based, mapped, relative), label/field/annotation/phase conditions,
  dry-run, per-policy rate limiting and batching.
- Controller with leader election, Prometheus metrics, Kubernetes events, and
  validating/mutating admission webhooks.
