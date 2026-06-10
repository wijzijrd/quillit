import {
  User, MapPin, Shield, CalendarDays, Package, BookMarked,
  Skull, Star, Scroll, Swords, Crown, Flame, Globe, Gem,
  MessageSquare, Footprints, Home, Landmark, Church,
  type LucideComponent,
} from 'lucide-vue-next'

export const ICON_REGISTRY: Record<string, LucideComponent> = {
  User, MapPin, Shield, CalendarDays, Package, BookMarked,
  Skull, Star, Scroll, Swords, Crown, Flame, Globe, Gem,
  MessageSquare, Footprints, Home, Landmark, Church,
}

export const AVAILABLE_ICONS = Object.keys(ICON_REGISTRY)

export function resolveIcon(name: string): LucideComponent {
  return ICON_REGISTRY[name] ?? BookMarked
}
