// Grouped by what a reader is doing: learning what jmapc is, writing a
// request, or calling the code generated from one.
export default {
  index: {
    title: 'Home',
    display: 'hidden',
    theme: {
      layout: 'full',
      sidebar: false,
      toc: false,
      breadcrumb: false,
      pagination: false,
      timestamp: false,
      copyPage: false
    }
  },
  '-- overview': { type: 'separator', title: 'Overview' },
  introduction: 'Introduction',
  why: 'Why jmapc',
  'getting-started': 'Getting started',
  '-- requests': { type: 'separator', title: 'Requests' },
  requests: 'Writing a request',
  properties: 'Property sets',
  verification: 'Verification',
  run: 'Sending a request',
  '-- generated': { type: 'separator', title: 'Generated code' },
  'generated-code': 'Generated names',
  errors: 'Errors',
  push: 'Push',
  paging: 'Paging',
  blobs: 'Blobs',
  client: 'Configuring the client',
  testing: 'Testing',
  languages: 'Rust and TypeScript',
  '-- reference': { type: 'separator', title: 'Reference' },
  cli: 'The jmapc command',
  extensions: 'Vendor extensions',
  coverage: 'Coverage',
  contributing: 'Working on jmapc'
}
