import { Composition } from "remotion";
import { Video } from "./Video";
import { FPS, T } from "./theme";

export function Root() {
  return <Composition id="Hero" component={Video} width={1600} height={1000} fps={FPS} durationInFrames={T.end} />;
}
