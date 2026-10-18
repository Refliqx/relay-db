//! Tauri commands = the API surface the frontend sees.
//!
//! RULE: one command = one operation meaningful to the user. There is no
//! "generic passthrough" command (`invoke_engine(method, params)`) because that
//! would defeat the point of the restriction: the frontend could call any
//! method without a clear contract. Every new feature = a new command.

mod connection;
mod export;
mod schema;
mod system;

pub use connection::*;
pub use export::*;
pub use schema::*;
pub use system::*;
