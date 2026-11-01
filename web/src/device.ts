const browsers: [RegExp, string][] = [
  [/Edg\//, 'Edge'],
  [/Firefox\//, 'Firefox'],
  [/Chrome\//, 'Chrome'],
  [/Safari\//, 'Safari'],
]

const systems: [RegExp, string][] = [
  [/iPhone/, 'an iPhone'],
  [/iPad/, 'an iPad'],
  [/Android/, 'Android'],
  [/Mac OS X|Macintosh/, 'a Mac'],
  [/Windows/, 'Windows'],
  [/Linux/, 'Linux'],
]

export function describeDevice(userAgent?: string) {
  if (!userAgent) return 'An unknown device'
  const browser = browsers.find(([re]) => re.test(userAgent))?.[1]
  const system = systems.find(([re]) => re.test(userAgent))?.[1]
  if (browser && system) return `${browser} on ${system}`
  return browser ?? system ?? 'An unknown device'
}
