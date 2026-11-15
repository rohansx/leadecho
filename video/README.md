# Hero video

Remotion project for the landing page hero video. Separate package; not part of the dashboard build.

- Install: `pnpm install --ignore-workspace`
- Preview while editing: `pnpm studio`
- Render: `pnpm render` writes mp4, webm and the poster to `out/` and copies them to `../dashboard/public/hero-demo.*`
- Scene timings (frames, 30fps) and colours: `src/theme.ts`. Colours mirror the `.dark` tokens in `dashboard/src/globals.css`.
- Scenes: `src/scenes-a.tsx` (stream, funnel), `src/scenes-b.tsx` (thread, person), `src/scenes-c.tsx` (draft, outcome). Shell and captions: `src/Video.tsx`.
- All data is fictional sample data. Do not add real people, companies or posts.
- The first and last frames are the bare app shell, so the video loops. Keep that when changing timings.
- Files too big? Add `--scale=0.8` to the render commands in `package.json` (gives 1280x800).
