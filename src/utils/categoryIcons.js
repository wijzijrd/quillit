import {
  User, MapPin, Shield, CalendarDays, Package, BookMarked,
  Skull, Star, Scroll, Swords, Crown, Flame, Globe, Gem,
  MessageSquare, Footprints, Home, Landmark, Church,
} from 'lucide-vue-next'

export const ICON_REGISTRY = {
  User, MapPin, Shield, CalendarDays, Package, BookMarked,
  Skull, Star, Scroll, Swords, Crown, Flame, Globe, Gem,
  MessageSquare, Footprints, Home, Landmark, Church,
}

export const AVAILABLE_ICONS = Object.keys(ICON_REGISTRY)

export function resolveIcon(name) {
  return ICON_REGISTRY[name] ?? BookMarked
}
