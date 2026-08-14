// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 alibaba/open-code-review Contributors

import { useEffect, useState, useTransition } from 'react';
import { useLocation, type Location } from 'react-router-dom';

/**
 * Returns a location that trails `useLocation()` by one React transition.
 *
 * Rendering `<Routes location={...}>` from this value keeps the previous
 * route on screen while the next route's lazy chunk downloads: the location
 * update is applied inside `startTransition`, so a suspending route keeps
 * the current content visible instead of unmounting to the `<Suspense>`
 * fallback. The fallback still shows on first paint, when there is no
 * previous content to keep.
 */
export function useTransitionedLocation(): Location {
  const location = useLocation();
  const [displayLocation, setDisplayLocation] = useState(location);
  const [, startTransition] = useTransition();

  useEffect(() => {
    startTransition(() => {
      setDisplayLocation(location);
    });
  }, [location, startTransition]);

  return displayLocation;
}
