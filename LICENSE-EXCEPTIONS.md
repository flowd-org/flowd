# License Exceptions for flowd

flowd is licensed under the **GNU Affero General Public License**, version 3 or any later version,
i.e. **AGPL-3.0-or-later** (SPDX identifier: `AGPL-3.0-or-later`).

Under section 7 of the AGPL, the flowd project grants the following **Additional Permissions**
(“Exceptions”). These Additional Permissions are intended to be **narrow, minimal, and compatible
with OSI principles**, and they only relax certain effects of the AGPL — they do not reduce any
obligations to publish source code of modified versions of flowd itself when required by the AGPL.

If this file and the AGPL license text appear to conflict, the AGPL text controls, except where
these Additional Permissions explicitly grant *more* permission as allowed by AGPL §7.

---

## 1. Outputs Exception

**Intent:** Using flowd to run jobs and generate runtime outputs, logs, or artifacts **does not**
by itself cause those outputs to become subject to the copyleft obligations of the AGPL.

**Permission:**

To the extent that the AGPL could be interpreted to require that the **runtime outputs** of a
Covered Work be licensed under the AGPL or distributed with source code:

- The authors of flowd permit you to treat:
  - logs,
  - metrics,
  - job outputs,
  - artifacts,
  - result files, and
  - other data **produced by running flowd** on your own inputs or scripts,

as **not** being part of the “Corresponding Source” or “Covered Work” solely by virtue of having
been produced by flowd.

You may therefore license and distribute such outputs under terms of your choice, provided that:

- You do not incorporate portions of flowd’s own source code, documentation, or other copyrighted
  material into those outputs beyond what is permitted by applicable law and fair use.
- You remain responsible for any third-party licenses that apply to your own inputs, scripts,
  data, or dependencies.

This exception does **not**:

- Exempt you from the AGPL requirements for distributing modified versions of flowd itself.
- Change your obligations for derivative works of flowd or for works that include flowd code.

---

## 2. Job-Script Boundary Exception

**Intent:** Merely **using** flowd via CLI/HTTP/JSON to run user-supplied scripts or job payloads
does **not**, by itself, make those scripts or payloads a derivative work of flowd.

**Permission:**

When you use flowd, including any of its CLIs, APIs, or configuration mechanisms, to:

- submit, execute, or orchestrate **user-supplied scripts, binaries, or job definitions**, or
- send or receive **JSON, YAML, or other structured data** as inputs or outputs,

the authors of flowd grant you permission to treat those **user-supplied scripts, binaries, job
definitions, and data** as **separate works**, not as derivative works of flowd, solely because:

- they are executed by, transmitted through, or orchestrated by flowd, or
- they use flowd’s CLI, HTTP, or JSON interfaces as documented.

As a result:

- The AGPL’s copyleft obligations for flowd do **not** automatically apply to those user-supplied
  scripts, binaries, or data merely because they are run or orchestrated by flowd.

This exception does **not**:

- Alter the status of any work that actually includes or links to flowd’s source code.
- Change the copyleft obligations that apply if you modify flowd itself or create a work that
  is a derivative of flowd under applicable copyright law.

---

## 3. External Extensions Boundary Exception

**Intent:** Implementations of extensions that run **out-of-process** and communicate only via
documented CLI/HTTP/JSON interfaces are treated as **separate programs**, not as derivative works
of flowd solely because they integrate with it.

**Permission:**

If you implement an extension, integration, adapter, plugin, or service that:

1. Runs as a **separate process** or service from flowd (i.e. not linked to flowd as a library and
   not built into the same binary), and
2. Communicates with flowd **exclusively** through:
   - documented CLI interfaces (invoking flowd as a subprocess), and/or
   - documented HTTP or JSON-based APIs or protocols,

then, to the extent permitted by AGPL §7, the authors of flowd grant you permission to treat that
extension/integration as a **separate work**, not as a derivative work of flowd **solely** because
of that form of communication.

As a result:

- The AGPL’s copyleft obligations for flowd do **not** automatically apply to such an external
  extension/integration merely due to its protocol-level interaction with flowd.

This exception does **not**:

- Change the obligations that apply if your extension incorporates, modifies, or statically/dynamically
  links against flowd’s source code or non-trivial portions of it.
- Change the AGPL requirements that apply if you distribute a modified version of flowd itself or a
  combined work that is a derivative of flowd.

---

## 4. Scope and Modifications

These Additional Permissions:

- Apply only to flowd and its official upstream distributions.
- Do not grant any trademark rights.
- Are themselves licensed under the same terms as flowd, but may be removed or modified by downstream
  redistributors **only** if they clearly mark such changes and avoid misrepresenting which permissions
  apply to their version.

If you redistribute a modified version of flowd and choose to remove or alter these Additional
Permissions, you MUST:

1. Change the name of your distribution in a way that clearly distinguishes it from the upstream
   flowd project; and
2. Clearly document which, if any, of these Additional Permissions still apply.

---

## 5. No legal advice

This file is intended to express the upstream authors’ Additional Permissions under AGPL §7 in a
clear, developer-friendly form. It is **not** legal advice.

If you have questions about how these permissions apply to your particular use case, you should seek
advice from a qualified lawyer.
