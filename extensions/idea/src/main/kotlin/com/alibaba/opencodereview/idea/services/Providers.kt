// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

package com.alibaba.opencodereview.idea.services

import java.util.Locale

// The host needs only names to route configuration to providers or custom_providers.
private val PRESET_PROVIDER_NAMES: Set<String> = generatedPresetProviderNames()

/** Uses Locale.ROOT so provider-name normalization is independent of the system locale. */
fun isPresetProvider(name: String): Boolean =
    PRESET_PROVIDER_NAMES.contains(name.trim().lowercase(Locale.ROOT))

/** Returns a defensive copy for tests and diagnostics. */
fun presetProviderNames(): Set<String> = PRESET_PROVIDER_NAMES.toSet()
