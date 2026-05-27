# Frontend Architecture Decisions

## 2026-05-27: Keep Backend Contracts Stable

The first redesign pass should not change API routes or storage. The user pain is mostly interaction density and information architecture.

## 2026-05-27: Use Existing CSS First

Use a Material-inspired app shell, rail, contained list, modal, and sticky action bar with local CSS and `lucide-react` icons for the initial pass. Adding Material UI can still happen later if component complexity grows, but the current app can reach the target workflow without taking on a new dependency immediately.

## 2026-05-27: Component Extraction Later

The immediate goal is a working contained workbench. After behavior is verified, split `main.tsx` into focused components if the file becomes difficult to maintain.

