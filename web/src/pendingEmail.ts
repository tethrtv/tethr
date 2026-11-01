let email = ''
let joining = false

export const pendingEmail = {
  get: () => email,
  joining: () => joining,
  set: (value: string, isJoining = false) => {
    email = value
    joining = isJoining
  },
}
