# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [0.7.3] (2026-05-19)

Initial tagged release.

Provides a cumulative memory budget for parsing untrusted input,
extracted from seehuhn.de/go/sfnt and shared across the Quire modules
that need to bound peak allocations on adversarial files.

### Added
- `Budget` with `New`, `Charge`, and `Available`; safe for concurrent use.
- `AllocSlice[T]` generic helper that charges and allocates in one step,
  with overflow-safe size computation.
- `ErrExceeded` sentinel returned when a charge would push the budget
  below zero.
- Per-allocation surcharge to prevent amplification via many tiny
  allocations.
- `MapEntryOverhead` constant for callers charging map insertions.
