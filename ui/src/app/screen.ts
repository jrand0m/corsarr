// Props every routed screen receives from App.svelte.
export interface ScreenProps {
  /** Path params (":id"). */
  params?: Record<string, string>;
  /** Screen id inside the area ("movies", "add"). */
  screen?: string;
  /** Access screens: the session ended (show "sign in again"). */
  expired?: boolean;
  /** Access screens: call after the session exists. */
  onsignedin?: () => void;
}
