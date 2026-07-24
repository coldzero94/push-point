// The detail screen IS the inspector (11 §0 — detail + edit merged). The
// /links/$id route is bookmark/deep-link entry: it renders the inspector for the
// path id and returns to the list on close. The everyday path opens the same
// inspector as a ?link overlay on the list (see LinkInspector).
export { LinkInspectorRoute as LinkDetailScreen } from './LinkInspector'
