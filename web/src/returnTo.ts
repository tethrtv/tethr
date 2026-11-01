const key = 'tethr.returnTo'

export const returnTo = {
  set(path: string) {
    try {
      localStorage.setItem(key, path)
    } catch {}
  },
  take() {
    try {
      const path = localStorage.getItem(key)
      localStorage.removeItem(key)
      if (path?.startsWith('/') && !path.startsWith('//')) return path
    } catch {}
    return '/'
  },
}
