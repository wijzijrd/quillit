import { useBreakpoints, breakpointsTailwind } from '@vueuse/core'

// Single source of truth for responsive layout decisions. Mobile layouts are
// a future feature; components should branch on these flags rather than
// duplicating media queries so the shell can be rearranged per breakpoint.
export function useBreakpoint() {
  const breakpoints = useBreakpoints(breakpointsTailwind)
  return {
    isMobile: breakpoints.smaller('md'),
    isDesktop: breakpoints.greaterOrEqual('md'),
    breakpoints,
  }
}
